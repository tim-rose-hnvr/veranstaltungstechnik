package kern

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// Platzaufbau beschreibt einen Sitzplatz samt der Kamera, die ihn zeigt.
// Der Aufbau kommt aus der Datenbank und ändert sich zur Laufzeit nicht.
type Platzaufbau struct {
	Nummer        int
	Name          string
	KameraName    string
	KameraAdresse string
	Kanal         uint8
	Preset        uint8
}

// Kamerasteuerung ruft einen in der Kamera gespeicherten Preset ab.
// Die Kamera hält die Position, der Server nur die Nummer.
type Kamerasteuerung interface {
	PresetAbrufen(ctx context.Context, adresse string, kanal, preset uint8) error
}

// Ereignisschreiber hängt ein Ereignis an die Kette des Saals an.
type Ereignisschreiber interface {
	EreignisAnfuegen(ctx context.Context, saalID, art string, nutzlast map[string]any) (Ereignis, error)
}

// PlatzZustand ist ein Sitzplatz, wie ihn der Client sieht.
type PlatzZustand struct {
	Nummer int    `json:"nummer"`
	Name   string `json:"name"`
	Mikro  bool   `json:"mikro"`
	Belegt bool   `json:"belegt"`
}

// Kamerazustand ist die zuletzt ausgelöste Kamerabewegung.
type Kamerazustand struct {
	Name       string `json:"name"`
	Preset     uint8  `json:"preset"`
	Erreichbar bool   `json:"erreichbar"`
}

// Zustand ist der vollständige Zustand des Saals. Der Server sendet immer
// alles, nie Teiländerungen — dann heilen sich Aussetzer von selbst.
type Zustand struct {
	Typ      string         `json:"typ"`
	Stand    uint64         `json:"stand"`
	MaxOffen int            `json:"max_offen"`
	Plaetze  []PlatzZustand `json:"plaetze"`
	Kamera   *Kamerazustand `json:"kamera"`
}

type platz struct {
	aufbau Platzaufbau
	mikro  bool
	belegt bool
}

// Kern hält den Zustand eines Saals und setzt die Regeln durch.
type Kern struct {
	saalID    string
	maxOffen  int
	zeitlimit time.Duration
	steuerung Kamerasteuerung
	ablage    Ereignisschreiber
	protokoll *slog.Logger

	mu      sync.Mutex
	plaetze []*platz
	nach    map[int]*platz
	stand   uint64
	kamera  *Kamerazustand

	melder func(Zustand)

	// wartend zählt laufende Kamerabefehle, damit Tests darauf warten können.
	wartend sync.WaitGroup
}

// Neu baut den Kern für einen Saal auf.
func Neu(saalID string, aufbau []Platzaufbau, maxOffen int, zeitlimit time.Duration,
	steuerung Kamerasteuerung, ablage Ereignisschreiber, protokoll *slog.Logger) (*Kern, error) {

	if maxOffen < 1 {
		return nil, fmt.Errorf("max_offene_mikrofone muss mindestens 1 sein, ist %d", maxOffen)
	}
	if protokoll == nil {
		protokoll = slog.Default()
	}

	k := &Kern{
		saalID:    saalID,
		maxOffen:  maxOffen,
		zeitlimit: zeitlimit,
		steuerung: steuerung,
		ablage:    ablage,
		protokoll: protokoll,
		nach:      make(map[int]*platz, len(aufbau)),
		melder:    func(Zustand) {},
	}
	for _, a := range aufbau {
		if _, doppelt := k.nach[a.Nummer]; doppelt {
			return nil, fmt.Errorf("platz %d kommt doppelt vor", a.Nummer)
		}
		p := &platz{aufbau: a}
		k.plaetze = append(k.plaetze, p)
		k.nach[a.Nummer] = p
	}
	sort.Slice(k.plaetze, func(i, j int) bool {
		return k.plaetze[i].aufbau.Nummer < k.plaetze[j].aufbau.Nummer
	})
	return k, nil
}

// SetzeMelder hinterlegt die Funktion, die nach jeder Änderung den
// vollständigen Zustand verteilt. Vor dem Start aufrufen.
func (k *Kern) SetzeMelder(melder func(Zustand)) {
	if melder != nil {
		k.melder = melder
	}
}

// Zustand liefert den aktuellen Zustand, etwa für einen neuen Client.
func (k *Kern) Zustand() Zustand {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.zustandIntern()
}

// Anmelden belegt einen Platz.
func (k *Kern) Anmelden(ctx context.Context, nummer int) error {
	return k.aendern(ctx, nummer, "platz_angemeldet", func(p *platz) *Fehler {
		if p.belegt {
			return fehler(CodePlatzBelegt, fmt.Sprintf("Platz %d ist bereits belegt", nummer))
		}
		p.belegt = true
		return nil
	})
}

// Abmelden gibt einen Platz wieder frei. Ein offenes Mikrofon wird dabei
// geschlossen, sonst bliebe es nach dem Verschwinden des Geräts offen.
func (k *Kern) Abmelden(ctx context.Context, nummer int) error {
	k.mu.Lock()
	p, bekannt := k.nach[nummer]
	if !bekannt {
		k.mu.Unlock()
		return fehler(CodePlatzUnbekannt, fmt.Sprintf("Platz %d gibt es nicht", nummer))
	}
	if !p.belegt && !p.mikro {
		k.mu.Unlock()
		return nil
	}
	nutzlast := map[string]any{"platz": nummer, "mikro_war_offen": p.mikro}
	if err := k.schreiben(ctx, "platz_abgemeldet", nutzlast); err != nil {
		k.mu.Unlock()
		return err
	}
	p.belegt = false
	p.mikro = false
	k.stand++
	z := k.zustandIntern()
	k.mu.Unlock()

	k.melder(z)
	return nil
}

// MikroAn öffnet ein Mikrofon und fährt die zuständige Kamera auf den Preset
// des Platzes. Mehr als max_offene_mikrofone gleichzeitig ist nicht möglich.
func (k *Kern) MikroAn(ctx context.Context, nummer int) error {
	k.mu.Lock()
	p, bekannt := k.nach[nummer]
	if !bekannt {
		k.mu.Unlock()
		return fehler(CodePlatzUnbekannt, fmt.Sprintf("Platz %d gibt es nicht", nummer))
	}
	if p.mikro {
		k.mu.Unlock()
		return nil
	}
	if offen := k.offeneIntern(); offen >= k.maxOffen {
		k.mu.Unlock()
		return fehler(CodeGrenzeErreicht, fmt.Sprintf("%d von %d Mikrofonen offen", offen, k.maxOffen))
	}
	if err := k.schreiben(ctx, "mikro_an", map[string]any{"platz": nummer}); err != nil {
		k.mu.Unlock()
		return err
	}
	p.mikro = true
	k.stand++
	aufbau := p.aufbau
	z := k.zustandIntern()
	k.mu.Unlock()

	k.melder(z)

	// Der Kamerabefehl läuft nebenher. Ein nicht erreichbares Gerät darf die
	// Antwort an die Clients nicht um das Zeitlimit verzögern.
	k.wartend.Add(1)
	go func() {
		defer k.wartend.Done()
		k.kameraFahren(aufbau)
	}()
	return nil
}

// MikroAus schließt ein Mikrofon.
func (k *Kern) MikroAus(ctx context.Context, nummer int) error {
	return k.aendern(ctx, nummer, "mikro_aus", func(p *platz) *Fehler {
		if !p.mikro {
			return nil // bereits zu, keine Änderung
		}
		p.mikro = false
		return nil
	})
}

// KameraAbwarten wartet auf laufende Kamerabefehle. Nur für Tests.
func (k *Kern) KameraAbwarten() { k.wartend.Wait() }

// aendern führt eine Zustandsänderung durch: prüfen, Ereignis schreiben,
// Zustand setzen, verteilen. Meldet die Prüfung nil und ändert nichts, wird
// kein Ereignis geschrieben.
func (k *Kern) aendern(ctx context.Context, nummer int, art string, pruefen func(*platz) *Fehler) error {
	k.mu.Lock()
	p, bekannt := k.nach[nummer]
	if !bekannt {
		k.mu.Unlock()
		return fehler(CodePlatzUnbekannt, fmt.Sprintf("Platz %d gibt es nicht", nummer))
	}

	vorher := *p
	if f := pruefen(p); f != nil {
		*p = vorher
		k.mu.Unlock()
		return f
	}
	if *p == vorher {
		k.mu.Unlock()
		return nil
	}

	if err := k.schreiben(ctx, art, map[string]any{"platz": nummer}); err != nil {
		*p = vorher
		k.mu.Unlock()
		return err
	}
	k.stand++
	z := k.zustandIntern()
	k.mu.Unlock()

	k.melder(z)
	return nil
}

func (k *Kern) kameraFahren(a Platzaufbau) {
	// Ohne Cancel des auslösenden Aufrufs: der Befehl gehört zum Saal, nicht
	// zur Verbindung, die ihn ausgelöst hat.
	ctx, abbrechen := context.WithTimeout(context.Background(), k.zeitlimit)
	defer abbrechen()

	art := "kamera_preset_abgerufen"
	erreichbar := true
	nutzlast := map[string]any{
		"platz":  a.Nummer,
		"kamera": a.KameraName,
		"preset": int(a.Preset),
	}

	if err := k.steuerung.PresetAbrufen(ctx, a.KameraAdresse, a.Kanal, a.Preset); err != nil {
		// Ein Kameraausfall ist kein Systemfehler: protokollieren und weiter.
		art = "kamera_nicht_erreichbar"
		erreichbar = false
		nutzlast["grund"] = err.Error()
		k.protokoll.Warn("kamera nicht erreichbar",
			"kamera", a.KameraName, "adresse", a.KameraAdresse, "platz", a.Nummer, "grund", err)
	}

	k.mu.Lock()
	if err := k.schreiben(context.Background(), art, nutzlast); err != nil {
		k.mu.Unlock()
		k.protokoll.Error("kameraereignis nicht geschrieben", "art", art, "grund", err)
		return
	}
	k.kamera = &Kamerazustand{Name: a.KameraName, Preset: a.Preset, Erreichbar: erreichbar}
	k.stand++
	z := k.zustandIntern()
	k.mu.Unlock()

	k.melder(z)
}

// schreiben hängt ein Ereignis an die Kette. Es wird vor der Zustandsänderung
// aufgerufen: einen Zustandswechsel ohne Protokolleintrag darf es nicht geben.
// Aufrufer hält k.mu.
func (k *Kern) schreiben(ctx context.Context, art string, nutzlast map[string]any) *Fehler {
	if k.ablage == nil {
		return nil
	}
	// Das Schreiben überlebt eine abgebrochene Client-Verbindung.
	ctx, abbrechen := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer abbrechen()

	if _, err := k.ablage.EreignisAnfuegen(ctx, k.saalID, art, nutzlast); err != nil {
		k.protokoll.Error("ereignis nicht geschrieben", "art", art, "grund", err)
		return fehler(CodeSpeicherFehler, "Ereignis konnte nicht protokolliert werden")
	}
	return nil
}

// offeneIntern zählt offene Mikrofone. Aufrufer hält k.mu.
func (k *Kern) offeneIntern() int {
	offen := 0
	for _, p := range k.plaetze {
		if p.mikro {
			offen++
		}
	}
	return offen
}

// zustandIntern baut den Zustandsschnappschuss. Aufrufer hält k.mu.
func (k *Kern) zustandIntern() Zustand {
	z := Zustand{
		Typ:      "zustand",
		Stand:    k.stand,
		MaxOffen: k.maxOffen,
		Plaetze:  make([]PlatzZustand, 0, len(k.plaetze)),
		Kamera:   k.kamera,
	}
	for _, p := range k.plaetze {
		z.Plaetze = append(z.Plaetze, PlatzZustand{
			Nummer: p.aufbau.Nummer,
			Name:   p.aufbau.Name,
			Mikro:  p.mikro,
			Belegt: p.belegt,
		})
	}
	return z
}
