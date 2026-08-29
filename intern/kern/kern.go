package kern

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Platzaufbau beschreibt einen Sitzplatz samt der Kamera, die ihn zeigt.
// Der Aufbau kommt aus der Datenbank und ändert sich zur Laufzeit nicht.
type Platzaufbau struct {
	Nummer int
	Name   string
	// Reihe und Spalte sind die Sitzordnung im Saal. Leer: keine Geometrie
	// hinterlegt, die Oberfläche zeigt dann eine Kachelreihe.
	Reihe         string
	Spalte        int
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

// Ablage ist alles, was der Kern dauerhaft festhalten muss.
type Ablage interface {
	Abstimmungsablage
	Tagesordnungsablage
	EreignisAnfuegen(ctx context.Context, saalID, art string, nutzlast map[string]any) (Ereignis, error)
	SitzungZustandSetzen(ctx context.Context, sitzungID string, zustand Sitzungszustand, zeit time.Time) error
	TeilnahmeZustandSetzen(ctx context.Context, teilnahmeID string, zustand Teilnahmezustand) error
	WortmeldungAnlegen(ctx context.Context, sitzungID, teilnahmeID string) (string, int64, error)
	WortmeldungZustandSetzen(ctx context.Context, wortmeldungID string, zustand Wortzustand) error
}

// PlatzZustand ist ein Sitzplatz, wie ihn der Client sieht.
type PlatzZustand struct {
	Nummer  int    `json:"nummer"`
	Name    string `json:"name"`
	Person  string `json:"person"`
	Mikro   bool   `json:"mikro"`
	Belegt  bool   `json:"belegt"`
	HatWort bool   `json:"hat_wort"`
	Reihe   string `json:"reihe,omitempty"`
	Spalte  int    `json:"spalte,omitempty"`
	// Widerspruch gegen die Aufzeichnung. Steht im Zustand, damit die
	// Oberfläche erklären kann, warum die Kamera nicht fährt.
	Widerspruch bool `json:"widerspruch"`
}

// Kamerazustand ist die zuletzt ausgelöste Kamerabewegung.
type Kamerazustand struct {
	Name       string `json:"name"`
	Preset     uint8  `json:"preset"`
	Erreichbar bool   `json:"erreichbar"`
}

// Zustand ist der gemeinsame Zustand des Saals. Der Server sendet immer
// alles, nie Teiländerungen — dann heilen sich Aussetzer von selbst.
type Zustand struct {
	Typ          string             `json:"typ"`
	Stand        uint64             `json:"stand"`
	MaxOffen     int                `json:"max_offen"`
	Aufzeichnung bool               `json:"aufzeichnung"`
	Sitzung      SitzungZustand     `json:"sitzung"`
	Plaetze      []PlatzZustand     `json:"plaetze"`
	Tagesordnung []TopZustand       `json:"tagesordnung"`
	Redeliste    []RedelisteEintrag `json:"redeliste"`
	Abstimmung   *AbstimmungZustand `json:"abstimmung"`
	Kamera       *Kamerazustand     `json:"kamera"`
}

type platz struct {
	aufbau    Platzaufbau
	mikro     bool
	belegt    bool
	teilnahme *Teilnahmeaufbau
}

// Aufbau ist alles, was der Kern beim Start braucht.
type Aufbau struct {
	SaalID         string
	Saal           string // Name des Saals, für die Beschriftung des Saalplans
	SitzungID      string
	Titel          string
	SitzungZustand Sitzungszustand
	Beginn         *time.Time // Nullpunkt der Zeitachse, nil solange nicht eröffnet
	Plaetze        []Platzaufbau
	Teilnahmen     []Teilnahmeaufbau
	Tagesordnung   []Tagesordnungspunkt
	Unterlagen     []Unterlage
	Wortmeldungen  []Wortmeldung // offene aus der Datenbank
	Abstimmung     *Abstimmung   // laufende oder zuletzt beendete
	LeitungPlatz   int           // 0: aus der Rolle ableiten
	MaxOffen       int
	Zeitlimit      time.Duration
}

// Kern hält den Zustand einer Sitzung und setzt die Regeln durch.
// Jede Rechteprüfung liegt hier — nie in der Oberfläche.
type Kern struct {
	saalID    string
	saal      string
	sitzungID string
	titel     string
	maxOffen  int
	zeitlimit time.Duration
	steuerung Kamerasteuerung
	ablage    Ablage
	protokoll *slog.Logger

	mu           sync.Mutex
	sitzung      Sitzungszustand
	beginn       time.Time // Nullpunkt der Zeitachse, Nullwert: noch nicht eröffnet
	plaetze      []*platz
	nach         map[int]*platz
	tagesordnung []*Tagesordnungspunkt
	unterlagen   []*Unterlage
	freigaben    map[string]*freigabe
	aufzeichnung bool
	leitungPlatz int
	redeliste    []*Wortmeldung
	abstimmung   *Abstimmung
	stand        uint64
	kamera       *Kamerazustand

	melder  func()
	wartend sync.WaitGroup
}

// Neu baut den Kern auf.
func Neu(a Aufbau, steuerung Kamerasteuerung, ablage Ablage, protokoll *slog.Logger) (*Kern, error) {
	if a.MaxOffen < 1 {
		return nil, fmt.Errorf("max_offene_mikrofone muss mindestens 1 sein, ist %d", a.MaxOffen)
	}
	if protokoll == nil {
		protokoll = slog.Default()
	}
	if a.SitzungZustand == "" {
		a.SitzungZustand = SitzungVorbereitet
	}

	k := &Kern{
		saalID:    a.SaalID,
		saal:      a.Saal,
		sitzungID: a.SitzungID,
		titel:     a.Titel,
		maxOffen:  a.MaxOffen,
		zeitlimit: a.Zeitlimit,
		steuerung: steuerung,
		ablage:    ablage,
		protokoll: protokoll,
		sitzung:   a.SitzungZustand,
		nach:      make(map[int]*platz, len(a.Plaetze)),
		freigaben: make(map[string]*freigabe),
		melder:    func() {},
	}

	if a.Beginn != nil {
		k.beginn = *a.Beginn
	}

	for _, aufbau := range a.Plaetze {
		if _, doppelt := k.nach[aufbau.Nummer]; doppelt {
			return nil, fmt.Errorf("platz %d kommt doppelt vor", aufbau.Nummer)
		}
		p := &platz{aufbau: aufbau}
		k.plaetze = append(k.plaetze, p)
		k.nach[aufbau.Nummer] = p
	}
	sort.Slice(k.plaetze, func(i, j int) bool {
		return k.plaetze[i].aufbau.Nummer < k.plaetze[j].aufbau.Nummer
	})

	for i := range a.Teilnahmen {
		t := a.Teilnahmen[i]
		p, bekannt := k.nach[t.PlatzNummer]
		if !bekannt {
			return nil, fmt.Errorf("teilnahme verweist auf unbekannten platz %d", t.PlatzNummer)
		}
		if p.teilnahme != nil {
			return nil, fmt.Errorf("platz %d hat zwei teilnahmen", t.PlatzNummer)
		}
		p.teilnahme = &t
	}

	nummern := make(map[int]bool, len(a.Tagesordnung))
	for i := range a.Tagesordnung {
		t := a.Tagesordnung[i]
		if nummern[t.Nummer] {
			return nil, fmt.Errorf("tagesordnungspunkt %d kommt doppelt vor", t.Nummer)
		}
		nummern[t.Nummer] = true
		if t.Zustand == "" {
			t.Zustand = TopOffen
		}
		k.tagesordnung = append(k.tagesordnung, &t)
	}
	sort.Slice(k.tagesordnung, func(i, j int) bool {
		return k.tagesordnung[i].Nummer < k.tagesordnung[j].Nummer
	})
	laufend := 0
	for _, t := range k.tagesordnung {
		if t.Zustand == TopLaufend {
			laufend++
		}
	}
	if laufend > 1 {
		return nil, fmt.Errorf("%d tagesordnungspunkte stehen auf laufend, höchstens einer darf es", laufend)
	}
	k.aufzeichnung = k.aufzeichnungSollIntern()

	for i := range a.Unterlagen {
		u := a.Unterlagen[i]
		if u.Stufe == "" {
			u.Stufe = StufeIntern
		}
		k.unterlagen = append(k.unterlagen, &u)
	}
	sort.Slice(k.unterlagen, func(i, j int) bool {
		if k.unterlagen[i].TopNummer != k.unterlagen[j].TopNummer {
			return k.unterlagen[i].TopNummer < k.unterlagen[j].TopNummer
		}
		return k.unterlagen[i].Nummer < k.unterlagen[j].Nummer
	})

	for i := range a.Wortmeldungen {
		w := a.Wortmeldungen[i]
		if w.Zustand.Offen() {
			k.redeliste = append(k.redeliste, &w)
		}
	}
	sort.Slice(k.redeliste, func(i, j int) bool { return k.redeliste[i].FolgeNr < k.redeliste[j].FolgeNr })
	k.abstimmung = a.Abstimmung

	// Der Staffelstab kommt aus dem Ereignisprotokoll. Steht dort nichts,
	// führt die Teilnahme mit der Rolle leitung.
	k.leitungPlatz = a.LeitungPlatz
	if k.leitungPlatz == 0 {
		for _, p := range k.plaetze {
			if p.teilnahme != nil && p.teilnahme.Rolle == RolleLeitung {
				k.leitungPlatz = p.aufbau.Nummer
				break
			}
		}
	}
	return k, nil
}

// SetzeMelder hinterlegt die Funktion, die nach jeder Änderung verteilt.
func (k *Kern) SetzeMelder(melder func()) {
	if melder != nil {
		k.melder = melder
	}
}

// Zustand liefert den gemeinsamen Zustand.
func (k *Kern) Zustand() Zustand {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.zustandIntern()
}

// Ich liefert den verbindungseigenen Teil: wer ich bin und was ich darf.
// Für einen nicht angemeldeten Platz kommt nil.
func (k *Kern) Ich(nummer int) *IchZustand {
	k.mu.Lock()
	defer k.mu.Unlock()

	p, bekannt := k.nach[nummer]
	if !bekannt || p.teilnahme == nil {
		return nil
	}
	ich := &IchZustand{
		Platz:  nummer,
		Person: p.teilnahme.Person,
		Rolle:  p.teilnahme.Rolle,
		Darf:   make([]string, 0, len(alleAktionen)),
	}
	for _, aktion := range alleAktionen {
		if k.darfIntern(p, aktion) == nil {
			ich.Darf = append(ich.Darf, aktion)
		}
	}
	// Die Sitzungsmappe ist je Rolle verschieden und gehört deshalb hierhin
	// und nicht in den gemeinsamen Zustand.
	for _, u := range k.unterlagen {
		if !u.Stufe.DarfSehen(p.teilnahme.Rolle) {
			continue
		}
		ich.Unterlagen = append(ich.Unterlagen, UnterlageZustand{
			ID: u.ID, TopNummer: u.TopNummer, Titel: u.Titel, Dateiname: u.Dateiname,
			Typ: u.Typ, Groesse: u.Groesse, Stufe: u.Stufe,
		})
	}
	return ich
}

// Anmelden belegt einen Platz nach Prüfung der PIN.
func (k *Kern) Anmelden(ctx context.Context, nummer int, pin string) error {
	k.mu.Lock()
	p, bekannt := k.nach[nummer]
	if !bekannt {
		k.mu.Unlock()
		return fehler(CodePlatzUnbekannt, fmt.Sprintf("Platz %d gibt es nicht", nummer))
	}
	if p.teilnahme == nil {
		k.mu.Unlock()
		return fehler(CodeNichtBerechtigt, fmt.Sprintf("Für Platz %d ist niemand eingetragen", nummer))
	}
	if p.belegt {
		k.mu.Unlock()
		return fehler(CodePlatzBelegt, fmt.Sprintf("Platz %d ist bereits belegt", nummer))
	}
	if err := bcrypt.CompareHashAndPassword(p.teilnahme.PinHash, []byte(pin)); err != nil {
		k.mu.Unlock()
		// Kein Hinweis darauf, ob der Platz existiert oder die PIN falsch war
		// — die Antwort ist für beide Fälle dieselbe Länge wert.
		return fehler(CodePinFalsch, "PIN stimmt nicht")
	}

	if err := k.schreiben(ctx, "platz_angemeldet", map[string]any{
		"platz": nummer, "person": p.teilnahme.Person, "rolle": string(p.teilnahme.Rolle),
	}); err != nil {
		k.mu.Unlock()
		return err
	}
	if err := k.ablage.TeilnahmeZustandSetzen(ctx, p.teilnahme.ID, TeilnahmeAnwesend); err != nil {
		k.protokoll.Error("teilnahmezustand nicht gespeichert", "platz", nummer, "grund", err)
	}
	p.belegt = true
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// Abmelden gibt einen Platz frei. Ein offenes Mikrofon wird geschlossen.
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

	if err := k.schreiben(ctx, "platz_abgemeldet", map[string]any{
		"platz": nummer, "mikro_war_offen": p.mikro,
	}); err != nil {
		k.mu.Unlock()
		return err
	}
	// Wer geht, verschwindet auch aus der Redeliste — sonst erteilt die
	// Leitung das Wort an einen leeren Platz.
	if w := k.wortmeldungIntern(nummer); w != nil {
		neu := WortZurueckgezogen
		if w.Zustand != WortGemeldet {
			neu = WortBeendet
		}
		k.wortmeldungSetzenIntern(ctx, nummer, neu)
	}
	if p.teilnahme != nil {
		if err := k.ablage.TeilnahmeZustandSetzen(ctx, p.teilnahme.ID, TeilnahmeAbwesend); err != nil {
			k.protokoll.Error("teilnahmezustand nicht gespeichert", "platz", nummer, "grund", err)
		}
	}
	p.belegt = false
	p.mikro = false
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// SitzungEroeffnen setzt die Sitzung auf laufend.
func (k *Kern) SitzungEroeffnen(ctx context.Context, absender int) error {
	return k.sitzungSetzen(ctx, absender, AktionSitzungEroeffnen, SitzungLaufend,
		[]Sitzungszustand{SitzungVorbereitet, SitzungBereit, SitzungUnterbrochen}, "sitzung_eroeffnet")
}

// SitzungSchliessen beendet die Sitzung. Offene Mikrofone und Wortmeldungen
// werden dabei geschlossen.
func (k *Kern) SitzungSchliessen(ctx context.Context, absender int) error {
	return k.sitzungSetzen(ctx, absender, AktionSitzungSchliessen, SitzungGeschlossen,
		[]Sitzungszustand{SitzungLaufend, SitzungUnterbrochen}, "sitzung_geschlossen")
}

func (k *Kern) sitzungSetzen(ctx context.Context, absender int, aktion string,
	ziel Sitzungszustand, erlaubtVon []Sitzungszustand, art string) error {

	k.mu.Lock()
	if err := k.darfPlatz(absender, aktion); err != nil {
		k.mu.Unlock()
		return err
	}
	passt := false
	for _, z := range erlaubtVon {
		if k.sitzung == z {
			passt = true
		}
	}
	if !passt {
		k.mu.Unlock()
		return fehler(CodeSitzungLaeuftNicht,
			fmt.Sprintf("Die Sitzung ist %s, das geht hier nicht", k.sitzung))
	}

	// Der Nullpunkt der Zeitachse wird vor dem Ereignis gesetzt, damit die
	// Eröffnung selbst schon bei 0 ms liegt.
	vorherigerBeginn := k.beginn
	jetzt := time.Now()
	if ziel == SitzungLaufend && k.beginn.IsZero() {
		k.beginn = jetzt
	}

	if err := k.schreiben(ctx, art, map[string]any{"platz": absender, "zustand": string(ziel)}); err != nil {
		k.beginn = vorherigerBeginn
		k.mu.Unlock()
		return err
	}
	k.sitzung = ziel
	if err := k.ablage.SitzungZustandSetzen(ctx, k.sitzungID, ziel, jetzt); err != nil {
		k.protokoll.Error("sitzungszustand nicht gespeichert", "grund", err)
	}

	if ziel == SitzungGeschlossen {
		// Eine nicht ausgezählte Abstimmung ist kein Ergebnis. Sie bliebe
		// sonst für immer auf laufend stehen und ließe sich nie beenden.
		if k.abstimmung.Laeuft() {
			if err := k.schreiben(ctx, "abstimmung_abgebrochen", map[string]any{
				"abstimmung": k.abstimmung.ID, "titel": k.abstimmung.Titel,
				"grund": "sitzung geschlossen",
			}); err != nil {
				k.protokoll.Error("abbruch nicht protokolliert", "grund", err)
			}
			if err := k.ablage.AbstimmungZustandSetzen(ctx, k.abstimmung.ID,
				AbstimmungAbgebrochen, time.Now()); err != nil {
				k.protokoll.Error("abstimmung nicht gespeichert", "grund", err)
			}
			k.abstimmung.Zustand = AbstimmungAbgebrochen
		}
		for _, p := range k.plaetze {
			if p.mikro {
				p.mikro = false
			}
		}
		for _, w := range k.redeliste {
			if err := k.ablage.WortmeldungZustandSetzen(ctx, w.ID, WortBeendet); err != nil {
				k.protokoll.Error("wortmeldung nicht gespeichert", "grund", err)
			}
		}
		k.redeliste = nil
		// Ein aufgerufener Punkt bleibt sonst für immer laufend stehen.
		if t := k.laufenderTopIntern(); t != nil {
			if err := k.topSetzenIntern(ctx, t, TopAbgeschlossen, absender); err != nil {
				k.protokoll.Error("tagesordnungspunkt nicht abgeschlossen", "top", t.Nummer)
			}
		}
	}
	k.aufzeichnungPruefenIntern(ctx)
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// WortMelden trägt den Platz in die Redeliste ein.
func (k *Kern) WortMelden(ctx context.Context, absender int) error {
	k.mu.Lock()
	if err := k.darfPlatz(absender, AktionWortMelden); err != nil {
		k.mu.Unlock()
		return err
	}
	if w := k.wortmeldungIntern(absender); w != nil {
		k.mu.Unlock()
		return nil // steht schon in der Liste
	}

	p := k.nach[absender]
	id, folgeNr, err := k.ablage.WortmeldungAnlegen(ctx, k.sitzungID, p.teilnahme.ID)
	if err != nil {
		k.protokoll.Error("wortmeldung nicht angelegt", "platz", absender, "grund", err)
		k.mu.Unlock()
		return fehler(CodeSpeicherFehler, "Wortmeldung konnte nicht festgehalten werden")
	}
	if err := k.schreiben(ctx, "wort_gemeldet", map[string]any{"platz": absender, "folge_nr": folgeNr}); err != nil {
		k.mu.Unlock()
		return err
	}
	k.redeliste = append(k.redeliste, &Wortmeldung{
		ID: id, FolgeNr: folgeNr, PlatzNummer: absender, Zustand: WortGemeldet,
	})
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// WortZurueckziehen nimmt die eigene Meldung zurück, solange sie nur gemeldet ist.
func (k *Kern) WortZurueckziehen(ctx context.Context, absender int) error {
	return k.wortWechseln(ctx, absender, absender, AktionWortZurueckziehen,
		WortZurueckgezogen, "wort_zurueckgezogen")
}

// WortErteilen gibt einem Platz das Wort.
func (k *Kern) WortErteilen(ctx context.Context, absender, ziel int) error {
	return k.wortWechseln(ctx, absender, ziel, AktionWortErteilen, WortErteilt, "wort_erteilt")
}

// WortEntziehen nimmt das Wort wieder weg und schließt das Mikrofon.
func (k *Kern) WortEntziehen(ctx context.Context, absender, ziel int) error {
	return k.wortWechseln(ctx, absender, ziel, AktionWortEntziehen, WortEntzogen, "wort_entzogen")
}

func (k *Kern) wortWechseln(ctx context.Context, absender, ziel int, aktion string,
	neu Wortzustand, art string) error {

	k.mu.Lock()
	if err := k.darfPlatz(absender, aktion); err != nil {
		k.mu.Unlock()
		return err
	}
	if _, bekannt := k.nach[ziel]; !bekannt {
		k.mu.Unlock()
		return fehler(CodePlatzUnbekannt, fmt.Sprintf("Platz %d gibt es nicht", ziel))
	}

	w := k.wortmeldungIntern(ziel)
	if w == nil {
		k.mu.Unlock()
		return fehler(CodeKeinWort, fmt.Sprintf("Platz %d steht nicht auf der Redeliste", ziel))
	}
	if !WortUebergangErlaubt(w.Zustand, neu) {
		k.mu.Unlock()
		return fehler(CodeKeinWort,
			fmt.Sprintf("Aus %s wird nicht %s", w.Zustand, neu))
	}

	if err := k.schreiben(ctx, art, map[string]any{"platz": ziel, "von": absender}); err != nil {
		k.mu.Unlock()
		return err
	}
	k.wortmeldungSetzenIntern(ctx, ziel, neu)

	// Entzogenes Wort schließt das Mikrofon sofort.
	if neu == WortEntzogen {
		if p := k.nach[ziel]; p.mikro {
			p.mikro = false
		}
	}
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// MikroAn öffnet ein Mikrofon und fährt die Kamera auf den Preset des Platzes.
func (k *Kern) MikroAn(ctx context.Context, absender, ziel int) error {
	k.mu.Lock()
	p, bekannt := k.nach[ziel]
	if !bekannt {
		k.mu.Unlock()
		return fehler(CodePlatzUnbekannt, fmt.Sprintf("Platz %d gibt es nicht", ziel))
	}
	if err := k.darfPlatz(absender, AktionMikroAn); err != nil {
		k.mu.Unlock()
		return err
	}
	if err := k.darfFremdenPlatz(absender, ziel); err != nil {
		k.mu.Unlock()
		return err
	}
	// Wer nicht die aktive Leitung ist, braucht das erteilte Wort.
	if absender != k.leitungPlatz && !k.hatWortIntern(ziel) {
		k.mu.Unlock()
		return fehler(CodeKeinWort, "Das Wort wurde noch nicht erteilt")
	}
	if p.mikro {
		k.mu.Unlock()
		return nil
	}
	if offen := k.offeneIntern(); offen >= k.maxOffen {
		k.mu.Unlock()
		return fehler(CodeGrenzeErreicht, fmt.Sprintf("%d von %d Mikrofonen offen", offen, k.maxOffen))
	}

	if err := k.schreiben(ctx, "mikro_an", map[string]any{"platz": ziel, "von": absender}); err != nil {
		k.mu.Unlock()
		return err
	}
	p.mikro = true
	if w := k.wortmeldungIntern(ziel); w != nil && WortUebergangErlaubt(w.Zustand, WortLaufend) {
		k.wortmeldungSetzenIntern(ctx, ziel, WortLaufend)
	}
	k.stand++
	aufbau := p.aufbau
	// Während einer laufenden Abstimmung sind automatische Technikeingriffe
	// gesperrt. Das Mikrofon geht auf, die Kamera bleibt stehen.
	abstimmungLaeuft := k.abstimmung.Laeuft()
	// Wer der Aufzeichnung widersprochen hat, wird nicht angefahren. Das ist
	// keine Einstellung, die jemand vergessen kann, sondern hängt an der
	// Teilnahme.
	widerspruch := p.teilnahme != nil && p.teilnahme.Widerspruch
	if widerspruch {
		if err := k.schreiben(ctx, "kamera_uebersprungen", map[string]any{
			"platz": ziel, "grund": "widerspruch gegen die aufzeichnung",
		}); err != nil {
			k.protokoll.Error("übersprungene kamerafahrt nicht protokolliert", "platz", ziel)
		}
	}
	k.mu.Unlock()

	k.melder()

	if abstimmungLaeuft {
		k.protokoll.Info("kamera bleibt stehen, es wird abgestimmt", "platz", ziel)
		return nil
	}
	if widerspruch {
		k.protokoll.Info("kamera bleibt stehen, widerspruch gegen die aufzeichnung", "platz", ziel)
		return nil
	}

	// Der Kamerabefehl läuft nebenher: ein stummes Gerät darf die Antwort an
	// die Clients nicht um das Zeitlimit verzögern.
	k.wartend.Add(1)
	go func() {
		defer k.wartend.Done()
		k.kameraFahren(aufbau)
	}()
	return nil
}

// MikroAus schließt ein Mikrofon.
func (k *Kern) MikroAus(ctx context.Context, absender, ziel int) error {
	k.mu.Lock()
	p, bekannt := k.nach[ziel]
	if !bekannt {
		k.mu.Unlock()
		return fehler(CodePlatzUnbekannt, fmt.Sprintf("Platz %d gibt es nicht", ziel))
	}
	if err := k.darfPlatz(absender, AktionMikroAus); err != nil {
		k.mu.Unlock()
		return err
	}
	if err := k.darfFremdenPlatz(absender, ziel); err != nil {
		k.mu.Unlock()
		return err
	}
	if !p.mikro {
		k.mu.Unlock()
		return nil
	}

	if err := k.schreiben(ctx, "mikro_aus", map[string]any{"platz": ziel, "von": absender}); err != nil {
		k.mu.Unlock()
		return err
	}
	p.mikro = false
	k.wortmeldungSetzenIntern(ctx, ziel, WortBeendet)
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// LeitungUebergeben reicht den Staffelstab weiter. Berechtigt sind viele,
// aktiv ist genau eine — und die abgebende Seite verliert die Rechte sofort.
func (k *Kern) LeitungUebergeben(ctx context.Context, absender, ziel int) error {
	k.mu.Lock()
	if err := k.darfPlatz(absender, AktionLeitungUebergeben); err != nil {
		k.mu.Unlock()
		return err
	}
	p, bekannt := k.nach[ziel]
	if !bekannt {
		k.mu.Unlock()
		return fehler(CodePlatzUnbekannt, fmt.Sprintf("Platz %d gibt es nicht", ziel))
	}
	if p.teilnahme == nil || p.teilnahme.Rolle != RolleLeitung {
		k.mu.Unlock()
		return fehler(CodeNichtBerechtigt,
			fmt.Sprintf("Platz %d ist nicht zur Sitzungsleitung berechtigt", ziel))
	}
	if ziel == k.leitungPlatz {
		k.mu.Unlock()
		return nil
	}

	if err := k.schreiben(ctx, "leitung_uebergeben", map[string]any{
		"von": absender, "an": ziel,
	}); err != nil {
		k.mu.Unlock()
		return err
	}
	k.leitungPlatz = ziel
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// LeitungUebernehmen holt den Staffelstab, wenn der führende Platz verwaist
// ist — etwa weil das Gerät ausgefallen ist. Das System vollzieht das nie von
// selbst: eine berechtigte Person muss es ausdrücklich tun, und es steht im
// Protokoll.
func (k *Kern) LeitungUebernehmen(ctx context.Context, absender int) error {
	k.mu.Lock()
	if err := k.darfPlatz(absender, AktionLeitungUebernehmen); err != nil {
		k.mu.Unlock()
		return err
	}

	vorher := k.leitungPlatz
	if err := k.schreiben(ctx, "leitung_uebernommen", map[string]any{
		"an": absender, "von": vorher, "grund": "führender Platz verwaist",
	}); err != nil {
		k.mu.Unlock()
		return err
	}
	k.leitungPlatz = absender
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// KameraAbwarten wartet auf laufende Kamerabefehle. Nur für Tests.
func (k *Kern) KameraAbwarten() { k.wartend.Wait() }

// darfPlatz prüft Sitzungszustand, Teilnahme und Rolle. Aufrufer hält k.mu.
func (k *Kern) darfPlatz(nummer int, aktion string) *Fehler {
	p, bekannt := k.nach[nummer]
	if !bekannt || p.teilnahme == nil {
		return fehler(CodeNichtBerechtigt, "Für diesen Platz ist niemand angemeldet")
	}
	return k.darfIntern(p, aktion)
}

// darfIntern ist die eine Stelle, an der über Rechte entschieden wird.
// Aufrufer hält k.mu.
func (k *Kern) darfIntern(p *platz, aktion string) *Fehler {
	if p.teilnahme == nil {
		return fehler(CodeNichtBerechtigt, "Für diesen Platz ist niemand eingetragen")
	}

	// Das Eröffnen ist die einzige Aktion vor der laufenden Sitzung.
	if aktion != AktionSitzungEroeffnen && k.sitzung != SitzungLaufend {
		return fehler(CodeSitzungLaeuftNicht, "Es läuft keine Sitzung")
	}

	leitungAktiv := p.aufbau.Nummer == k.leitungPlatz
	if !rolleDarf(p.teilnahme.Rolle, leitungAktiv, aktion) {
		if p.teilnahme.Rolle == RolleLeitung && !leitungAktiv {
			return fehler(CodeNichtBerechtigt, "Die Sitzungsleitung liegt gerade bei einem anderen Platz")
		}
		return fehler(CodeNichtBerechtigt, fmt.Sprintf("Die Rolle %s darf das nicht", p.teilnahme.Rolle))
	}

	// Was der Abstimmungszustand zusätzlich erlaubt oder verbietet. Damit ist
	// das Feld "darf" im Zustand genau — der Client muss nichts ableiten.
	switch aktion {
	case AktionLeitungUebernehmen:
		// Nur wenn der führende Platz verwaist ist. Sonst bliebe der
		// Staffelstab kein Staffelstab.
		if p.aufbau.Nummer == k.leitungPlatz {
			return fehler(CodeNichtBerechtigt, "Dieser Platz führt bereits")
		}
		if fuehrend, bekannt := k.nach[k.leitungPlatz]; k.leitungPlatz != 0 && bekannt && fuehrend.belegt {
			return fehler(CodeNichtBerechtigt,
				fmt.Sprintf("Platz %d führt und ist besetzt — die Leitung wird übergeben, nicht übernommen",
					k.leitungPlatz))
		}
	case AktionSitzungEroeffnen:
		// Eröffnen geht nur aus einem Zustand heraus, der es zulässt — sonst
		// böte die Oberfläche einen Knopf an, der immer fehlschlägt.
		if k.sitzung != SitzungVorbereitet && k.sitzung != SitzungBereit &&
			k.sitzung != SitzungUnterbrochen {
			return fehler(CodeSitzungLaeuftNicht,
				fmt.Sprintf("Die Sitzung ist %s", k.sitzung))
		}
	case AktionTopAufrufen, AktionTopAbschliessen, AktionTopVertagen:
		if len(k.tagesordnung) == 0 {
			return fehler(CodeKeinTop, "Diese Sitzung hat keine Tagesordnung")
		}
		// Ein Punktwechsel während einer laufenden Abstimmung ließe den
		// Beschluss ohne Punkt zurück — erst auszählen, dann weiter.
		if k.abstimmung.Laeuft() {
			return fehler(CodeAbstimmungLaeuft,
				"Es wird abgestimmt — der Tagesordnungspunkt bleibt bis zum Auszählen stehen")
		}
		if aktion != AktionTopAufrufen && k.laufenderTopIntern() == nil {
			return fehler(CodeKeinTop, "Es ist kein Tagesordnungspunkt aufgerufen")
		}

	case AktionAbstimmungStarten:
		if k.abstimmung.Laeuft() {
			return fehler(CodeAbstimmungLaeuft, "Es läuft bereits eine Abstimmung")
		}
		// Ein Beschluss gehört an einen Tagesordnungspunkt. Wo es eine
		// Tagesordnung gibt, muss einer aufgerufen sein — sonst steht der
		// Beschluss später ohne Bezug im Protokoll.
		if len(k.tagesordnung) > 0 && k.laufenderTopIntern() == nil {
			return fehler(CodeKeinTop,
				"Erst einen Tagesordnungspunkt aufrufen, dann abstimmen")
		}
	case AktionAbstimmungBeenden, AktionAbstimmungAbbrechen:
		if !k.abstimmung.Laeuft() {
			return fehler(CodeKeineAbstimmung, "Es läuft keine Abstimmung")
		}
	case AktionAbstimmungFeststellen:
		if k.abstimmung == nil || k.abstimmung.Zustand != AbstimmungAusgezaehlt {
			return fehler(CodeKeineAbstimmung, "Es gibt kein ausgezähltes Ergebnis")
		}
	case AktionStimmeAbgeben:
		if !k.abstimmung.Laeuft() {
			return fehler(CodeKeineAbstimmung, "Es läuft keine Abstimmung")
		}
		if k.abstimmung.Abgegeben[p.aufbau.Nummer] {
			return fehler(CodeSchonAbgestimmt, "Für diesen Platz ist bereits eine Stimme abgegeben")
		}
	}
	return nil
}

// darfFremdenPlatz erlaubt nur der aktiven Leitung, an fremden Plätzen zu
// schalten. Aufrufer hält k.mu.
func (k *Kern) darfFremdenPlatz(absender, ziel int) *Fehler {
	if absender == ziel || absender == k.leitungPlatz {
		return nil
	}
	return fehler(CodeNichtBerechtigt, "Nur die Sitzungsleitung schaltet fremde Plätze")
}

// wortmeldungIntern findet die offene Wortmeldung eines Platzes.
// Aufrufer hält k.mu.
func (k *Kern) wortmeldungIntern(nummer int) *Wortmeldung {
	for _, w := range k.redeliste {
		if w.PlatzNummer == nummer && w.Zustand.Offen() {
			return w
		}
	}
	return nil
}

// wortmeldungSetzenIntern schreibt den neuen Zustand und räumt geschlossene
// Meldungen aus der Liste. Aufrufer hält k.mu.
func (k *Kern) wortmeldungSetzenIntern(ctx context.Context, nummer int, neu Wortzustand) {
	w := k.wortmeldungIntern(nummer)
	if w == nil || !WortUebergangErlaubt(w.Zustand, neu) {
		return
	}
	w.Zustand = neu
	if err := k.ablage.WortmeldungZustandSetzen(ctx, w.ID, neu); err != nil {
		k.protokoll.Error("wortmeldung nicht gespeichert", "platz", nummer, "grund", err)
	}
	if !neu.Offen() {
		uebrig := k.redeliste[:0]
		for _, e := range k.redeliste {
			if e != w {
				uebrig = append(uebrig, e)
			}
		}
		k.redeliste = uebrig
	}
}

// hatWortIntern sagt, ob ein Platz sprechen darf. Aufrufer hält k.mu.
func (k *Kern) hatWortIntern(nummer int) bool {
	w := k.wortmeldungIntern(nummer)
	return w != nil && (w.Zustand == WortErteilt || w.Zustand == WortLaufend)
}

func (k *Kern) kameraFahren(a Platzaufbau) {
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
	k.mu.Unlock()

	k.melder()
}

// schreiben hängt ein Ereignis an die Kette. Es wird vor der Zustandsänderung
// aufgerufen: einen Zustandswechsel ohne Protokolleintrag darf es nicht geben.
// Aufrufer hält k.mu.
func (k *Kern) schreiben(ctx context.Context, art string, nutzlast map[string]any) *Fehler {
	if k.ablage == nil {
		return nil
	}
	ctx, abbrechen := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer abbrechen()

	nutzlast = k.zeitachseAnhaengen(nutzlast)
	if _, err := k.ablage.EreignisAnfuegen(ctx, k.saalID, art, nutzlast); err != nil {
		k.protokoll.Error("ereignis nicht geschrieben", "art", art, "grund", err)
		return fehler(CodeSpeicherFehler, "Ereignis konnte nicht protokolliert werden")
	}
	return nil
}

// zeitachseAnhaengen ergänzt jede Nutzlast um die Sitzung und um die
// Millisekunden seit Sitzungsbeginn. Eine Zeitachse je Sitzung ist die
// Voraussetzung dafür, Aufzeichnung, Transkript und Protokoll später
// zusammenzubringen — nachträglich ist sie nicht herstellbar.
//
// Beides steht bewusst in der Nutzlast und nicht in einer eigenen Spalte:
// die Nutzlast geht in den Hash ein, damit ist die Zeitachse ohne Änderung
// der Hash-Regel fälschungssicher. Aufrufer hält k.mu.
func (k *Kern) zeitachseAnhaengen(nutzlast map[string]any) map[string]any {
	angereichert := make(map[string]any, len(nutzlast)+2)
	for schluessel, wert := range nutzlast {
		angereichert[schluessel] = wert
	}
	if k.sitzungID != "" {
		angereichert["sitzung"] = k.sitzungID
	}
	if !k.beginn.IsZero() {
		angereichert["ms"] = time.Since(k.beginn).Milliseconds()
	}
	return angereichert
}

// Beginn ist der Nullpunkt der Zeitachse. Nullwert: noch nicht eröffnet.
func (k *Kern) Beginn() time.Time {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.beginn
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
		Typ:          "zustand",
		Stand:        k.stand,
		MaxOffen:     k.maxOffen,
		Aufzeichnung: k.aufzeichnung,
		Sitzung: SitzungZustand{
			Saal:         k.saal,
			Titel:        k.titel,
			Zustand:      k.sitzung,
			LeitungPlatz: k.leitungPlatz,
		},
		Plaetze:      make([]PlatzZustand, 0, len(k.plaetze)),
		Tagesordnung: k.tagesordnungZustandIntern(),
		Redeliste:    make([]RedelisteEintrag, 0, len(k.redeliste)),
		Abstimmung:   k.abstimmungZustandIntern(),
		Kamera:       k.kamera,
	}
	if t := k.laufenderTopIntern(); t != nil {
		z.Sitzung.AktuellerTop = t.Nummer
	}
	for _, p := range k.plaetze {
		pz := PlatzZustand{
			Nummer:  p.aufbau.Nummer,
			Name:    p.aufbau.Name,
			Mikro:   p.mikro,
			Belegt:  p.belegt,
			HatWort: k.hatWortIntern(p.aufbau.Nummer),
			Reihe:   p.aufbau.Reihe,
			Spalte:  p.aufbau.Spalte,
		}
		if p.teilnahme != nil {
			pz.Person = p.teilnahme.Person
			pz.Widerspruch = p.teilnahme.Widerspruch
		}
		z.Plaetze = append(z.Plaetze, pz)
	}
	for _, w := range k.redeliste {
		eintrag := RedelisteEintrag{Platz: w.PlatzNummer, Zustand: w.Zustand}
		if p := k.nach[w.PlatzNummer]; p != nil && p.teilnahme != nil {
			eintrag.Person = p.teilnahme.Person
		}
		z.Redeliste = append(z.Redeliste, eintrag)
	}
	return z
}
