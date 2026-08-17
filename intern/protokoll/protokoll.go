// Paket protokoll macht aus der Ereigniskette ein lesbares Sitzungsprotokoll.
//
// Es erfindet nichts. Jede Zeile stammt aus einem Ereignis, und die Kette wird
// vorher nachgerechnet — steht sie nicht, sagt das Protokoll das im Kopf und
// nicht im Kleingedruckten.
package protokoll

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
)

// Kettenleser liefert die Ereigniskette eines Saals.
type Kettenleser interface {
	Ereignisse(ctx context.Context, saalID string) ([]kern.Ereignis, error)
}

// Schreiber baut das Protokoll.
type Schreiber struct {
	saalID  string
	saal    string
	ablage  Kettenleser
	plaetze map[int]string // Platznummer -> Name der Person
}

// Neu baut den Schreiber. plaetze ordnet Platznummern Personen zu.
func Neu(saalID, saal string, ablage Kettenleser, teilnahmen []kern.Teilnahmeaufbau) *Schreiber {
	plaetze := make(map[int]string, len(teilnahmen))
	for _, t := range teilnahmen {
		plaetze[t.PlatzNummer] = t.Person
	}
	return &Schreiber{saalID: saalID, saal: saal, ablage: ablage, plaetze: plaetze}
}

// Markdown schreibt das Protokoll einer Sitzung. sitzungID grenzt ein, welche
// Ereignisse dazugehören — die Kette läuft über den ganzen Saal.
func (s *Schreiber) Markdown(ctx context.Context, sitzungID, titel string) (string, error) {
	kette, err := s.ablage.Ereignisse(ctx, s.saalID)
	if err != nil {
		return "", fmt.Errorf("kette lesen: %w", err)
	}
	kettenfehler := kern.KettePruefen(kette)

	var b strings.Builder
	fmt.Fprintf(&b, "# Protokoll — %s\n\n", titel)
	fmt.Fprintf(&b, "Saal: %s  \n", s.saal)

	if kettenfehler != nil {
		fmt.Fprintf(&b, "\n> **Achtung:** Die Ereigniskette ist nicht in Ordnung (%v).\n"+
			"> Dieses Protokoll ist nicht beweiskräftig.\n\n", kettenfehler)
	}

	var eigene []kern.Ereignis
	for _, e := range kette {
		if wert, gefunden := e.Nutzlast["sitzung"]; gefunden {
			if text, passt := wert.(string); passt && text == sitzungID {
				eigene = append(eigene, e)
			}
		}
	}
	if len(eigene) == 0 {
		b.WriteString("\nZu dieser Sitzung liegt kein Ereignis vor.\n")
		return b.String(), nil
	}

	fmt.Fprintf(&b, "Beginn: %s  \n", eigene[0].Zeit.Local().Format("02.01.2006, 15:04 Uhr"))
	fmt.Fprintf(&b, "Einträge: %d, Kette nachgerechnet: %s\n\n",
		len(eigene), jaNein(kettenfehler == nil))

	b.WriteString("## Verlauf\n\n")
	b.WriteString("| Zeit | Vorgang |\n|---|---|\n")
	for _, e := range eigene {
		zeile := s.zeile(e)
		if zeile == "" {
			continue
		}
		fmt.Fprintf(&b, "| %s | %s |\n", s.uhrzeit(e), zeile)
	}

	if beschluesse := s.beschluesse(eigene); beschluesse != "" {
		b.WriteString("\n## Beschlüsse\n\n")
		b.WriteString(beschluesse)
	}

	b.WriteString("\n---\n\nAutomatisch aus dem Ereignisprotokoll erzeugt. " +
		"Der Wortlaut der Redebeiträge ist nicht enthalten — dafür fehlt das Transkript.\n")
	return b.String(), nil
}

// uhrzeit zählt ab Sitzungsbeginn. Was davor geschah — Anmeldungen etwa —
// liegt noch auf keiner Zeitachse und wird als „vorab" ausgewiesen, statt eine
// Uhrzeit in dieselbe Spalte zu mischen.
func (s *Schreiber) uhrzeit(e kern.Ereignis) string {
	ms, gefunden := millisekunden(e.Nutzlast["ms"])
	if !gefunden {
		return "vorab"
	}
	dauer := time.Duration(ms) * time.Millisecond
	return fmt.Sprintf("%02d:%02d", int(dauer.Minutes()), int(dauer.Seconds())%60)
}

// zeile macht aus einem Ereignis einen Satz. Was hier nicht steht, gehört
// nicht ins Protokoll — etwa jede einzelne Kamerafahrt.
func (s *Schreiber) zeile(e kern.Ereignis) string {
	platz := zahl(e.Nutzlast["platz"])
	wer := s.wer(platz)

	switch e.Art {
	case "sitzung_eroeffnet":
		return fmt.Sprintf("Die Sitzung wird durch %s eröffnet.", s.wer(zahl(e.Nutzlast["platz"])))
	case "sitzung_geschlossen":
		return fmt.Sprintf("Die Sitzung wird durch %s geschlossen.", s.wer(zahl(e.Nutzlast["platz"])))
	case "platz_angemeldet":
		return fmt.Sprintf("%s nimmt auf Platz %d Platz.", text(e.Nutzlast["person"]), platz)
	case "platz_abgemeldet":
		return fmt.Sprintf("%s verlässt Platz %d.", wer, platz)
	case "leitung_uebergeben":
		return fmt.Sprintf("Die Sitzungsleitung geht von %s auf %s über.",
			s.wer(zahl(e.Nutzlast["von"])), s.wer(zahl(e.Nutzlast["an"])))
	case "wort_gemeldet":
		return fmt.Sprintf("%s meldet sich zu Wort.", wer)
	case "wort_erteilt":
		return fmt.Sprintf("%s erhält das Wort.", wer)
	case "wort_entzogen":
		return fmt.Sprintf("%s wird das Wort entzogen.", wer)
	case "wort_zurueckgezogen":
		return fmt.Sprintf("%s zieht die Wortmeldung zurück.", wer)
	case "mikro_an":
		return fmt.Sprintf("%s spricht.", wer)

	case "abstimmung_gestartet":
		return fmt.Sprintf("Abstimmung „%s\" (%s) wird eröffnet. "+
			"Anwesend %d von %d Stimmberechtigten, Quorum %d.",
			text(e.Nutzlast["titel"]), text(e.Nutzlast["art"]),
			zahl(e.Nutzlast["anwesend"]), zahl(e.Nutzlast["stimmberechtigt"]),
			zahl(e.Nutzlast["quorum"]))
	case "abstimmung_ausgezaehlt":
		return fmt.Sprintf("Abstimmung „%s\" ausgezählt: %d Ja, %d Nein, %d Enthaltungen.",
			text(e.Nutzlast["titel"]), zahl(e.Nutzlast["ja"]),
			zahl(e.Nutzlast["nein"]), zahl(e.Nutzlast["enthaltung"]))
	case "abstimmung_festgestellt":
		return fmt.Sprintf("Ergebnis festgestellt: „%s\" ist %s.",
			text(e.Nutzlast["titel"]), angenommen(e.Nutzlast["angenommen"]))
	case "abstimmung_abgebrochen":
		return fmt.Sprintf("Abstimmung „%s\" abgebrochen.", text(e.Nutzlast["titel"]))

	default:
		// mikro_aus, Kameraereignisse und Stimmabgaben stehen im
		// Ereignisprotokoll, aber nicht im Sitzungsprotokoll.
		return ""
	}
}

// beschluesse fasst die festgestellten Ergebnisse zusammen.
func (s *Schreiber) beschluesse(kette []kern.Ereignis) string {
	var b strings.Builder
	nummer := 0
	for _, e := range kette {
		if e.Art != "abstimmung_festgestellt" {
			continue
		}
		nummer++
		fmt.Fprintf(&b, "**%d. %s**  \n", nummer, text(e.Nutzlast["titel"]))
		fmt.Fprintf(&b, "%d Ja, %d Nein, %d Enthaltungen — %s.\n\n",
			zahl(e.Nutzlast["ja"]), zahl(e.Nutzlast["nein"]), zahl(e.Nutzlast["enthaltung"]),
			angenommen(e.Nutzlast["angenommen"]))
	}
	return b.String()
}

func (s *Schreiber) wer(platz int) string {
	if name, gefunden := s.plaetze[platz]; gefunden && name != "" {
		return name
	}
	if platz == 0 {
		return "unbekannt"
	}
	return fmt.Sprintf("Platz %d", platz)
}

func text(wert any) string {
	if s, passt := wert.(string); passt {
		return s
	}
	return ""
}

func zahl(wert any) int {
	ms, _ := millisekunden(wert)
	return int(ms)
}

// millisekunden liest eine Zahl unabhängig davon, ob sie direkt aus dem Kern
// kommt (int64) oder den Umweg über jsonb genommen hat (float64).
func millisekunden(wert any) (int64, bool) {
	switch zahl := wert.(type) {
	case int64:
		return zahl, true
	case int:
		return int64(zahl), true
	case float64:
		return int64(zahl), true
	default:
		return 0, false
	}
}

func angenommen(wert any) string {
	if ja, passt := wert.(bool); passt && ja {
		return "angenommen"
	}
	return "abgelehnt"
}

func jaNein(ja bool) string {
	if ja {
		return "ja"
	}
	return "nein"
}
