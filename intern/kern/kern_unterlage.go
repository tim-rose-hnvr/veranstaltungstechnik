package kern

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"
)

// FreigabeDauer ist die Lebenszeit einer Marke. Kurz genug, dass ein
// weitergegebener Verweis nichts nützt; lang genug für einen langsamen Abruf.
const FreigabeDauer = 30 * time.Second

type freigabe struct {
	unterlage     *Unterlage
	platz         int
	person        string
	gueltigBis    time.Time
	wasserzeichen string
}

// UnterlagenFuer liefert die Sitzungsmappe aus der Sicht eines Platzes.
// Was die Rolle nicht sehen darf, fehlt in der Liste — es wird nicht
// ausgegraut, sondern gar nicht erst genannt.
func (k *Kern) UnterlagenFuer(nummer int) []UnterlageZustand {
	k.mu.Lock()
	defer k.mu.Unlock()

	p, bekannt := k.nach[nummer]
	if !bekannt || p.teilnahme == nil {
		return nil
	}
	sichtbar := make([]UnterlageZustand, 0, len(k.unterlagen))
	for _, u := range k.unterlagen {
		if !u.Stufe.DarfSehen(p.teilnahme.Rolle) {
			continue
		}
		sichtbar = append(sichtbar, UnterlageZustand{
			ID: u.ID, TopNummer: u.TopNummer, Titel: u.Titel, Dateiname: u.Dateiname,
			Typ: u.Typ, Groesse: u.Groesse, Stufe: u.Stufe,
		})
	}
	return sichtbar
}

// UnterlageAbrufen prüft das Recht, schreibt den Zugriff in die Kette und gibt
// eine kurzlebige Marke aus. Das Zugriffsprotokoll ist die Kette selbst — eine
// zweite Liste daneben könnte auseinanderlaufen.
func (k *Kern) UnterlageAbrufen(ctx context.Context, absender int, unterlageID string) (*Freigabe, error) {
	k.mu.Lock()

	p, bekannt := k.nach[absender]
	if !bekannt || p.teilnahme == nil {
		k.mu.Unlock()
		return nil, fehler(CodeNichtBerechtigt, "Für diesen Platz ist niemand angemeldet")
	}
	var u *Unterlage
	for _, kandidat := range k.unterlagen {
		if kandidat.ID == unterlageID {
			u = kandidat
			break
		}
	}
	if u == nil {
		k.mu.Unlock()
		return nil, fehler(CodeUnterlageUnbekannt, "Diese Unterlage gibt es nicht")
	}
	if !u.Stufe.DarfSehen(p.teilnahme.Rolle) {
		// Der abgewiesene Versuch steht im Protokoll. Wer eine vertrauliche
		// Unterlage anfragt, die er nicht sehen darf, ist ein Vorgang.
		if err := k.schreiben(ctx, "unterlage_verweigert", map[string]any{
			"unterlage": u.ID, "titel": u.Titel, "stufe": string(u.Stufe),
			"platz": absender, "rolle": string(p.teilnahme.Rolle),
		}); err != nil {
			k.protokoll.Error("verweigerung nicht protokolliert", "unterlage", u.ID)
		}
		k.mu.Unlock()
		return nil, fehler(CodeNichtBerechtigt,
			fmt.Sprintf("Die Unterlage ist als %s eingestuft", u.Stufe))
	}

	// Das Wasserzeichen benennt den Empfänger. Es macht eine Weitergabe nicht
	// unmöglich, aber nachvollziehbar — und das ist der Zweck.
	wasserzeichen := fmt.Sprintf("%s · Platz %d · %s · %s",
		p.teilnahme.Person, absender, k.titel, time.Now().Format("02.01.2006 15:04"))

	if err := k.schreiben(ctx, "unterlage_geoeffnet", map[string]any{
		"unterlage": u.ID, "titel": u.Titel, "stufe": string(u.Stufe),
		"top": u.TopNummer, "platz": absender, "person": p.teilnahme.Person,
	}); err != nil {
		k.mu.Unlock()
		return nil, err
	}

	marke, err := marke()
	if err != nil {
		k.mu.Unlock()
		k.protokoll.Error("marke nicht erzeugbar", "grund", err)
		return nil, fehler(CodeSpeicherFehler, "Die Unterlage konnte nicht freigegeben werden")
	}
	k.freigaben[marke] = &freigabe{
		unterlage: u, platz: absender, person: p.teilnahme.Person,
		gueltigBis: time.Now().Add(FreigabeDauer), wasserzeichen: wasserzeichen,
	}
	k.freigabenAufraeumenIntern()
	k.mu.Unlock()

	return &Freigabe{
		Marke: marke, Unterlage: u.ID, Titel: u.Titel, Dateiname: u.Dateiname,
		Typ: u.Typ, Wasserzeichen: wasserzeichen,
	}, nil
}

// UnterlageOeffnen löst eine Marke ein und liefert die Datei. Die Marke gilt
// einmal: der zweite Abruf findet sie nicht mehr.
//
// Vor der Ausgabe wird die Prüfsumme nachgerechnet. Wurde die Datei unter dem
// laufenden System ausgetauscht, wird nicht ausgeliefert — eine Unterlage, die
// nicht mehr die eingelesene ist, gehört nicht in eine Sitzung.
func (k *Kern) UnterlageOeffnen(marke string) (io.ReadCloser, *Unterlage, string, error) {
	k.mu.Lock()
	f, bekannt := k.freigaben[marke]
	if bekannt {
		delete(k.freigaben, marke)
	}
	k.freigabenAufraeumenIntern()
	k.mu.Unlock()

	if !bekannt {
		return nil, nil, "", fehler(CodeFreigabeAbgelaufen, "Diese Freigabe gibt es nicht mehr")
	}
	if time.Now().After(f.gueltigBis) {
		return nil, nil, "", fehler(CodeFreigabeAbgelaufen, "Die Freigabe ist abgelaufen")
	}

	pruefsumme, err := DateiPruefsumme(f.unterlage.Datei)
	if err != nil {
		k.protokoll.Error("unterlage nicht lesbar", "datei", f.unterlage.Datei, "grund", err)
		return nil, nil, "", fehler(CodeUnterlageUnbekannt, "Die Unterlage ist nicht lesbar")
	}
	if pruefsumme != f.unterlage.Pruefsumme {
		k.protokoll.Error("unterlage wurde ausgetauscht",
			"datei", f.unterlage.Datei, "erwartet", f.unterlage.Pruefsumme, "gefunden", pruefsumme)
		return nil, nil, "", fehler(CodeUnterlageVeraendert,
			"Die Datei ist nicht mehr die eingelesene und wird nicht ausgeliefert")
	}

	datei, err := os.Open(f.unterlage.Datei)
	if err != nil {
		return nil, nil, "", fehler(CodeUnterlageUnbekannt, "Die Unterlage ist nicht lesbar")
	}
	return datei, f.unterlage, f.wasserzeichen, nil
}

// freigabenAufraeumenIntern wirft abgelaufene Marken weg. Aufrufer hält k.mu.
func (k *Kern) freigabenAufraeumenIntern() {
	jetzt := time.Now()
	for m, f := range k.freigaben {
		if jetzt.After(f.gueltigBis) {
			delete(k.freigaben, m)
		}
	}
}

// marke erzeugt eine nicht erratbare Kennung.
func marke() (string, error) {
	roh := make([]byte, 24)
	if _, err := rand.Read(roh); err != nil {
		return "", err
	}
	return hex.EncodeToString(roh), nil
}

// DateiPruefsumme bildet den SHA-256 einer Datei als Hex.
func DateiPruefsumme(pfad string) (string, error) {
	datei, err := os.Open(pfad)
	if err != nil {
		return "", err
	}
	defer datei.Close()

	h := sha256.New()
	if _, err := io.Copy(h, datei); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
