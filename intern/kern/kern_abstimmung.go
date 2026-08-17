package kern

import (
	"context"
	"fmt"
	"time"
)

// Abstimmungsablage hält Abstimmungen und Stimmen fest.
type Abstimmungsablage interface {
	AbstimmungAnlegen(ctx context.Context, sitzungID, titel string, art Abstimmungsart) (string, int64, error)
	AbstimmungStarten(ctx context.Context, abstimmungID string, stimmberechtigt, anwesend, quorum int, zeit time.Time) error
	AbstimmungZustandSetzen(ctx context.Context, abstimmungID string, zustand Abstimmungszustand, zeit time.Time) error
	StimmeAbgeben(ctx context.Context, abstimmungID, teilnahmeID string, wahl Wahl, geheim bool) error
}

// AbstimmungStarten eröffnet eine Abstimmung. Beschlussfähigkeit wird jetzt
// geprüft und eingefroren — wer später kommt, ändert sie nicht mehr.
func (k *Kern) AbstimmungStarten(ctx context.Context, absender int, titel string, art Abstimmungsart) error {
	if titel == "" {
		return fehler(CodeKeineAbstimmung, "Eine Abstimmung braucht einen Titel")
	}

	k.mu.Lock()
	if err := k.darfPlatz(absender, AktionAbstimmungStarten); err != nil {
		k.mu.Unlock()
		return err
	}
	if k.abstimmung != nil && k.abstimmung.Zustand == AbstimmungLaufend {
		k.mu.Unlock()
		return fehler(CodeAbstimmungLaeuft, "Es läuft bereits eine Abstimmung")
	}

	stimmberechtigt, anwesend := k.stimmenIntern()
	quorum := Quorum(stimmberechtigt)
	if anwesend < quorum {
		k.mu.Unlock()
		return fehler(CodeNichtBeschlussfaehig,
			fmt.Sprintf("Nicht beschlussfähig: %d von %d Stimmberechtigten anwesend, nötig sind %d",
				anwesend, stimmberechtigt, quorum))
	}

	id, folgeNr, err := k.ablage.AbstimmungAnlegen(ctx, k.sitzungID, titel, art)
	if err != nil {
		k.protokoll.Error("abstimmung nicht angelegt", "grund", err)
		k.mu.Unlock()
		return fehler(CodeSpeicherFehler, "Abstimmung konnte nicht angelegt werden")
	}

	jetzt := time.Now()
	nutzlast := map[string]any{
		"abstimmung": id, "titel": titel, "art": string(art),
		"stimmberechtigt": stimmberechtigt, "anwesend": anwesend, "quorum": quorum,
		"von": absender,
	}
	// Der Beschluss trägt den Tagesordnungspunkt, unter dem er gefasst wurde.
	// Nachträglich ist diese Zuordnung nicht mehr herstellbar.
	if t := k.laufenderTopIntern(); t != nil {
		nutzlast["top"] = t.Nummer
	}
	if err := k.schreiben(ctx, "abstimmung_gestartet", nutzlast); err != nil {
		k.mu.Unlock()
		return err
	}
	if err := k.ablage.AbstimmungStarten(ctx, id, stimmberechtigt, anwesend, quorum, jetzt); err != nil {
		k.protokoll.Error("abstimmung nicht gespeichert", "grund", err)
	}

	k.abstimmung = &Abstimmung{
		ID: id, FolgeNr: folgeNr, Titel: titel, Art: art, Zustand: AbstimmungLaufend,
		Stimmberechtigt: stimmberechtigt, Anwesend: anwesend, Quorum: quorum,
		Abgegeben: map[int]bool{}, Namentlich: map[int]Wahl{},
	}
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// StimmeAbgeben nimmt die Stimme des eigenen Platzes entgegen. Bei geheimer
// Wahl wird nur festgehalten, dass abgestimmt wurde — nie, wie.
func (k *Kern) StimmeAbgeben(ctx context.Context, absender int, wahl Wahl) error {
	k.mu.Lock()
	if err := k.darfPlatz(absender, AktionStimmeAbgeben); err != nil {
		k.mu.Unlock()
		return err
	}
	if k.abstimmung == nil || k.abstimmung.Zustand != AbstimmungLaufend {
		k.mu.Unlock()
		return fehler(CodeKeineAbstimmung, "Es läuft keine Abstimmung")
	}
	if k.abstimmung.Abgegeben[absender] {
		k.mu.Unlock()
		return fehler(CodeSchonAbgestimmt, "Für diesen Platz ist bereits eine Stimme abgegeben")
	}

	p := k.nach[absender]
	geheim := k.abstimmung.Art.Geheim()

	// Bei geheimer Wahl darf der Platz nicht in der Nutzlast stehen — sonst
	// entstünde im Protokoll genau die Zuordnung, die es nicht geben darf.
	nutzlast := map[string]any{"abstimmung": k.abstimmung.ID, "geheim": geheim}
	if !geheim {
		nutzlast["platz"] = absender
		nutzlast["wahl"] = string(wahl)
	}
	if err := k.schreiben(ctx, "stimme_abgegeben", nutzlast); err != nil {
		k.mu.Unlock()
		return err
	}
	if err := k.ablage.StimmeAbgeben(ctx, k.abstimmung.ID, p.teilnahme.ID, wahl, geheim); err != nil {
		k.protokoll.Error("stimme nicht gespeichert", "grund", err)
		k.mu.Unlock()
		return fehler(CodeSpeicherFehler, "Die Stimme konnte nicht festgehalten werden")
	}

	k.abstimmung.Abgegeben[absender] = true
	if !geheim {
		k.abstimmung.Namentlich[absender] = wahl
	}
	switch wahl {
	case WahlJa:
		k.abstimmung.Ja++
	case WahlNein:
		k.abstimmung.Nein++
	case WahlEnthaltung:
		k.abstimmung.Enthaltung++
	}
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// AbstimmungBeenden zählt aus. Erst jetzt wird das Ergebnis sichtbar.
func (k *Kern) AbstimmungBeenden(ctx context.Context, absender int) error {
	return k.abstimmungWechseln(ctx, absender, AktionAbstimmungBeenden,
		AbstimmungLaufend, AbstimmungAusgezaehlt, "abstimmung_ausgezaehlt")
}

// AbstimmungFeststellen macht das Ergebnis verbindlich.
func (k *Kern) AbstimmungFeststellen(ctx context.Context, absender int) error {
	return k.abstimmungWechseln(ctx, absender, AktionAbstimmungFeststellen,
		AbstimmungAusgezaehlt, AbstimmungFestgestellt, "abstimmung_festgestellt")
}

// AbstimmungAbbrechen verwirft eine laufende Abstimmung, etwa wenn die
// Beschlussfähigkeit verloren geht.
func (k *Kern) AbstimmungAbbrechen(ctx context.Context, absender int) error {
	return k.abstimmungWechseln(ctx, absender, AktionAbstimmungAbbrechen,
		AbstimmungLaufend, AbstimmungAbgebrochen, "abstimmung_abgebrochen")
}

func (k *Kern) abstimmungWechseln(ctx context.Context, absender int, aktion string,
	von, nach Abstimmungszustand, art string) error {

	k.mu.Lock()
	if err := k.darfPlatz(absender, aktion); err != nil {
		k.mu.Unlock()
		return err
	}
	if k.abstimmung == nil {
		k.mu.Unlock()
		return fehler(CodeKeineAbstimmung, "Es gibt keine Abstimmung")
	}
	if k.abstimmung.Zustand != von {
		k.mu.Unlock()
		return fehler(CodeKeineAbstimmung,
			fmt.Sprintf("Die Abstimmung ist %s, nicht %s", k.abstimmung.Zustand, von))
	}

	nutzlast := map[string]any{
		"abstimmung": k.abstimmung.ID, "titel": k.abstimmung.Titel, "von": absender,
	}
	if nach == AbstimmungAusgezaehlt || nach == AbstimmungFestgestellt {
		nutzlast["ja"] = k.abstimmung.Ja
		nutzlast["nein"] = k.abstimmung.Nein
		nutzlast["enthaltung"] = k.abstimmung.Enthaltung
		nutzlast["angenommen"] = k.abstimmung.Angenommen()
	}
	if err := k.schreiben(ctx, art, nutzlast); err != nil {
		k.mu.Unlock()
		return err
	}
	if err := k.ablage.AbstimmungZustandSetzen(ctx, k.abstimmung.ID, nach, time.Now()); err != nil {
		k.protokoll.Error("abstimmungszustand nicht gespeichert", "grund", err)
	}
	k.abstimmung.Zustand = nach
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// stimmenIntern zählt Stimmberechtigte und davon Anwesende.
// Aufrufer hält k.mu.
func (k *Kern) stimmenIntern() (stimmberechtigt, anwesend int) {
	for _, p := range k.plaetze {
		if p.teilnahme == nil || !p.teilnahme.Rolle.Stimmrecht() {
			continue
		}
		stimmberechtigt++
		if p.belegt {
			anwesend++
		}
	}
	return stimmberechtigt, anwesend
}

// abstimmungZustandIntern baut die Sicht des Clients. Solange die Abstimmung
// läuft, bleibt das Ergebnis leer. Aufrufer hält k.mu.
func (k *Kern) abstimmungZustandIntern() *AbstimmungZustand {
	if k.abstimmung == nil {
		return nil
	}
	a := k.abstimmung
	z := &AbstimmungZustand{
		Titel: a.Titel, Art: a.Art, Zustand: a.Zustand,
		Stimmberechtigt: a.Stimmberechtigt, Anwesend: a.Anwesend, Quorum: a.Quorum,
		Abgegeben: len(a.Abgegeben),
	}
	if a.Zustand == AbstimmungAusgezaehlt || a.Zustand == AbstimmungFestgestellt {
		ergebnis := &Ergebnis{
			Ja: a.Ja, Nein: a.Nein, Enthaltung: a.Enthaltung, Angenommen: a.Angenommen(),
		}
		if a.Art == AbstimmungNamentlich {
			ergebnis.Namentlich = make(map[int]Wahl, len(a.Namentlich))
			for platz, wahl := range a.Namentlich {
				ergebnis.Namentlich[platz] = wahl
			}
		}
		z.Ergebnis = ergebnis
	}
	return z
}
