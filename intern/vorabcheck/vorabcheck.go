// Paket vorabcheck prüft vor der Sitzung automatisch durch, was der Techniker
// sonst von Hand durchgeht: Kette, Plätze, Besetzung und jede Kamera.
//
// Geprüft wird nur, was das System heute wirklich kann. Ton und Video stehen
// hier nicht — ein Selbsttest, der Dinge grün meldet, die es nicht gibt, ist
// schlimmer als keiner.
package vorabcheck

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/siegel"
)

// ErrSitzungLaeuft meldet, dass der Vorabcheck abgelehnt wurde. Er fährt die
// Kameras an — das darf während einer laufenden Sitzung nicht passieren.
var ErrSitzungLaeuft = errors.New("die sitzung läuft, der vorabcheck fährt keine kameras")

// Ergebnis eines einzelnen Punktes.
type Ergebnis string

const (
	Ok      Ergebnis = "ok"
	Hinweis Ergebnis = "hinweis"
	Fehler  Ergebnis = "fehler"
)

// Punkt ist eine Zeile im Bericht. Der Text sagt, was zu tun ist — in der
// Sprache eines Kollegen, nicht in der eines Messgeräts.
type Punkt struct {
	Bereich  string   `json:"bereich"`
	Titel    string   `json:"titel"`
	Ergebnis Ergebnis `json:"ergebnis"`
	Text     string   `json:"text"`
}

// Bericht ist das Ergebnis eines Durchlaufs.
type Bericht struct {
	Zeit     time.Time `json:"zeit"`
	DauerMs  int64     `json:"dauer_ms"`
	Punkte   []Punkt   `json:"punkte"`
	Ok       int       `json:"ok"`
	Hinweise int       `json:"hinweise"`
	Fehler   int       `json:"fehler"`
	Bereit   bool      `json:"bereit"`
}

// Kettenleser liefert die Ereigniskette eines Saals.
type Kettenleser interface {
	Ereignisse(ctx context.Context, saalID string) ([]kern.Ereignis, error)
}

// Pruefer führt den Vorabcheck durch.
type Pruefer struct {
	aufbau    kern.Aufbau
	sitzung   *kern.Kern
	steuerung kern.Kamerasteuerung
	ablage    Kettenleser
	zeitlimit time.Duration
	jetzt     func() time.Time

	// siegelschluessel ist der öffentliche Teil des Siegelschlüssels. Ist er
	// nicht gesetzt, wird die Kette geprüft, aber nicht der Abschluss.
	siegelschluessel ed25519.PublicKey
}

// SetzeSiegelschluessel hinterlegt den Schlüssel, gegen den die Abschlüsse der
// Kette geprüft werden.
func (p *Pruefer) SetzeSiegelschluessel(oeffentlich ed25519.PublicKey) {
	p.siegelschluessel = oeffentlich
}

// Neu baut den Prüfer. aufbau ist der Stand beim Serverstart, sitzung liefert
// den laufenden Zustand.
func Neu(aufbau kern.Aufbau, sitzung *kern.Kern, steuerung kern.Kamerasteuerung,
	ablage Kettenleser, zeitlimit time.Duration) *Pruefer {

	if zeitlimit <= 0 {
		zeitlimit = 500 * time.Millisecond
	}
	return &Pruefer{
		aufbau:    aufbau,
		sitzung:   sitzung,
		steuerung: steuerung,
		ablage:    ablage,
		zeitlimit: zeitlimit,
		jetzt:     time.Now,
	}
}

// Laufen prüft alles durch. Während einer laufenden Sitzung wird abgelehnt.
func (p *Pruefer) Laufen(ctx context.Context) (Bericht, error) {
	zustand := p.sitzung.Zustand()
	if zustand.Sitzung.Zustand == kern.SitzungLaufend {
		return Bericht{}, ErrSitzungLaeuft
	}

	beginn := p.jetzt()
	bericht := Bericht{Zeit: beginn}

	bericht.Punkte = append(bericht.Punkte, p.sitzungPruefen(zustand)...)
	bericht.Punkte = append(bericht.Punkte, p.kettePruefen(ctx)...)
	bericht.Punkte = append(bericht.Punkte, p.plaetzePruefen(zustand)...)
	bericht.Punkte = append(bericht.Punkte, p.kamerasPruefen(ctx)...)

	for _, punkt := range bericht.Punkte {
		switch punkt.Ergebnis {
		case Ok:
			bericht.Ok++
		case Hinweis:
			bericht.Hinweise++
		case Fehler:
			bericht.Fehler++
		}
	}
	bericht.Bereit = bericht.Fehler == 0
	bericht.DauerMs = p.jetzt().Sub(beginn).Milliseconds()
	return bericht, nil
}

func (p *Pruefer) sitzungPruefen(z kern.Zustand) []Punkt {
	var punkte []Punkt

	switch z.Sitzung.Zustand {
	case kern.SitzungVorbereitet, kern.SitzungBereit:
		punkte = append(punkte, Punkt{"Sitzung", "Zustand", Ok,
			fmt.Sprintf("%q ist %s und kann eröffnet werden.", z.Sitzung.Titel, z.Sitzung.Zustand)})
	case kern.SitzungGeschlossen, kern.SitzungArchiviert:
		punkte = append(punkte, Punkt{"Sitzung", "Zustand", Fehler,
			fmt.Sprintf("%q ist %s und lässt sich nicht wieder eröffnen. "+
				"Für den nächsten Durchlauf einen neuen Titel in sitzung.json eintragen.",
				z.Sitzung.Titel, z.Sitzung.Zustand)})
	default:
		punkte = append(punkte, Punkt{"Sitzung", "Zustand", Hinweis,
			fmt.Sprintf("%q ist %s.", z.Sitzung.Titel, z.Sitzung.Zustand)})
	}

	// Genau eine aktive Leitung, und die muss auch besetzt sein können.
	berechtigt := 0
	for _, t := range p.aufbau.Teilnahmen {
		if t.Rolle == kern.RolleLeitung {
			berechtigt++
		}
	}
	switch {
	case z.Sitzung.LeitungPlatz == 0:
		punkte = append(punkte, Punkt{"Sitzung", "Sitzungsleitung", Fehler,
			"Keine Sitzungsleitung festgelegt. In sitzung.json braucht eine Teilnahme die Rolle leitung."})
	case berechtigt == 1:
		punkte = append(punkte, Punkt{"Sitzung", "Sitzungsleitung", Hinweis,
			fmt.Sprintf("Platz %d führt. Nur eine Person ist zur Leitung berechtigt — "+
				"eine Übergabe ist damit nicht möglich.", z.Sitzung.LeitungPlatz)})
	default:
		punkte = append(punkte, Punkt{"Sitzung", "Sitzungsleitung", Ok,
			fmt.Sprintf("Platz %d führt, %d Personen sind zur Leitung berechtigt.",
				z.Sitzung.LeitungPlatz, berechtigt)})
	}

	if p.aufbau.MaxOffen > len(p.aufbau.Plaetze) {
		punkte = append(punkte, Punkt{"Sitzung", "Offene Mikrofone", Hinweis,
			fmt.Sprintf("max_offene_mikrofone steht auf %d, es gibt aber nur %d Plätze.",
				p.aufbau.MaxOffen, len(p.aufbau.Plaetze))})
	} else {
		punkte = append(punkte, Punkt{"Sitzung", "Offene Mikrofone", Ok,
			fmt.Sprintf("Höchstens %d von %d Mikrofonen gleichzeitig offen.",
				p.aufbau.MaxOffen, len(p.aufbau.Plaetze))})
	}
	return punkte
}

func (p *Pruefer) kettePruefen(ctx context.Context) []Punkt {
	kette, err := p.ablage.Ereignisse(ctx, p.aufbau.SaalID)
	if err != nil {
		return []Punkt{{"Protokoll", "Ereigniskette", Fehler,
			"Die Kette ist nicht lesbar: " + err.Error()}}
	}
	if err := kern.KettePruefen(kette); err != nil {
		return []Punkt{{"Protokoll", "Ereigniskette", Fehler,
			fmt.Sprintf("Die Kette ist nicht in Ordnung: %v. Das Protokoll dieser Sitzung "+
				"ist nicht mehr beweiskräftig — vor der Sitzung klären.", err)}}
	}
	punkte := []Punkt{{"Protokoll", "Ereigniskette", Ok,
		fmt.Sprintf("%d Einträge, lückenlos und nachgerechnet.", len(kette))}}

	// Die Kette zeigt, dass niemand einen Eintrag geändert hat. Erst das
	// Siegel zeigt, dass niemand die ganze Kette neu gerechnet hat.
	if p.siegelschluessel == nil {
		return append(punkte, Punkt{"Protokoll", "Siegel", Hinweis,
			"Es ist kein Siegelschlüssel hinterlegt. Die Kette ist in sich stimmig, " +
				"aber ein vollständiger Nachbau fiele nicht auf."})
	}
	bericht := siegel.Pruefen(p.aufbau.SaalID, kette, p.siegelschluessel)
	switch {
	case !bericht.Ok():
		return append(punkte, Punkt{"Protokoll", "Siegel", Fehler,
			fmt.Sprintf("Ein Abschluss geht nicht auf: %s. Das Protokoll ist nicht "+
				"beweiskräftig — vor der Sitzung klären.", strings.Join(bericht.Fehler, "; "))})
	case bericht.Siegel == 0:
		return append(punkte, Punkt{"Protokoll", "Siegel", Hinweis,
			"Die Kette ist noch nie abgeschlossen worden. Der erste Abschluss kommt " +
				"beim Herunterfahren oder zur eingestellten Uhrzeit."})
	default:
		return append(punkte, Punkt{"Protokoll", "Siegel", Ok,
			fmt.Sprintf("%d Abschlüsse, gedeckt bis Eintrag %d von %d, Schlüssel %s.",
				bericht.Siegel, bericht.Gedeckt, bericht.Laenge,
				strings.Join(bericht.Fingerabdruecke, ", "))})
	}
}

func (p *Pruefer) plaetzePruefen(z kern.Zustand) []Punkt {
	var punkte []Punkt

	personAuf := make(map[int]kern.Teilnahmeaufbau, len(p.aufbau.Teilnahmen))
	for _, t := range p.aufbau.Teilnahmen {
		personAuf[t.PlatzNummer] = t
	}

	for _, platz := range p.aufbau.Plaetze {
		bereich := fmt.Sprintf("Platz %d", platz.Nummer)

		teilnahme, besetzt := personAuf[platz.Nummer]
		if !besetzt {
			punkte = append(punkte, Punkt{bereich, "Besetzung", Hinweis,
				fmt.Sprintf("%s ist keiner Person zugeordnet. Wer hier sitzt, kann sich "+
					"nicht anmelden — in sitzung.json ergänzen.", platz.Name)})
		} else {
			punkte = append(punkte, Punkt{bereich, "Besetzung", Ok,
				fmt.Sprintf("%s, %s.", teilnahme.Person, teilnahme.Rolle)})
		}

		switch {
		case platz.KameraName == "":
			punkte = append(punkte, Punkt{bereich, "Kamerazuordnung", Fehler,
				"Keine Kamera zugeordnet. Die Nachführung überspringt diesen Platz."})
		case platz.Preset == 0:
			punkte = append(punkte, Punkt{bereich, "Preset", Fehler,
				"Keine Presetnummer hinterlegt. In saal.json eintragen und in der Kamera speichern."})
		}
	}
	return punkte
}

// kamerasPruefen fährt jeden Platz an. Das prüft in einem Schritt, ob die
// Kamera erreichbar ist und ob der Preset in ihr gespeichert wurde.
func (p *Pruefer) kamerasPruefen(ctx context.Context) []Punkt {
	var punkte []Punkt

	for _, platz := range p.aufbau.Plaetze {
		if platz.KameraName == "" || platz.Preset == 0 {
			continue // steht schon als Fehler im Bericht
		}
		bereich := fmt.Sprintf("Platz %d", platz.Nummer)

		frist, abbrechen := context.WithTimeout(ctx, p.zeitlimit)
		err := p.steuerung.PresetAbrufen(frist, platz.KameraAdresse, platz.Kanal, platz.Preset)
		abbrechen()

		if err != nil {
			punkte = append(punkte, Punkt{bereich, "Kamera", Fehler,
				fmt.Sprintf("%s unter %s antwortet nicht. Kabel, Adresse und Stromversorgung prüfen.",
					platz.KameraName, platz.KameraAdresse)})
			continue
		}
		punkte = append(punkte, Punkt{bereich, "Kamera", Ok,
			fmt.Sprintf("%s ist auf Preset %d gefahren. Bild am Platz prüfen.",
				platz.KameraName, platz.Preset)})
	}
	return punkte
}
