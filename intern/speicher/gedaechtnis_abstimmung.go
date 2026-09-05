package speicher

import (
	"context"
	"fmt"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
)

type gAbstimmung struct {
	id              string
	sitzungID       string
	folgeNr         int64
	titel           string
	art             kern.Abstimmungsart
	zustand         kern.Abstimmungszustand
	stimmberechtigt int
	anwesend        int
	quorum          int
	// abgegeben hält fest, DASS eine Teilnahme abgestimmt hat.
	abgegeben map[string]bool
	// stimmen ist die Zählung bei offener und namentlicher Wahl — dort ist
	// die Zuordnung zur Person gewollt.
	stimmen []gStimme
	// zaehler ist die Zählung bei geheimer Wahl: nur Summen, keine
	// Einzelstimmen. Die Reihenfolge des Eintreffens ist damit weg, und mit
	// ihr die Möglichkeit, über sie auf die Person zu schließen. Dieselbe
	// Regel wie in der Datenbank, siehe migrationen/007_geheime_wahl.sql —
	// die beiden Ablagen müssen sich gleich verhalten, sonst zeigt ein Test
	// gegen die eine nicht, was die andere tut.
	zaehler map[kern.Wahl]int
}

type gStimme struct {
	teilnahmeID string
	wahl        kern.Wahl
}

// AbstimmungAnlegen legt eine Abstimmung an.
func (g *Gedaechtnis) AbstimmungAnlegen(ctx context.Context, sitzungID, titel string,
	art kern.Abstimmungsart) (string, int64, error) {

	g.mu.Lock()
	defer g.mu.Unlock()

	var hoechste int64
	for _, a := range g.abstimmungen {
		if a.sitzungID == sitzungID && a.folgeNr > hoechste {
			hoechste = a.folgeNr
		}
	}
	a := &gAbstimmung{
		id: g.id(), sitzungID: sitzungID, folgeNr: hoechste + 1, titel: titel, art: art,
		zustand: kern.AbstimmungVorbereitet, abgegeben: map[string]bool{},
	}
	g.abstimmungen = append(g.abstimmungen, a)
	return a.id, a.folgeNr, nil
}

// AbstimmungStarten friert Beschlussfähigkeit und Quorum ein.
func (g *Gedaechtnis) AbstimmungStarten(ctx context.Context, abstimmungID string,
	stimmberechtigt, anwesend, quorum int, zeit time.Time) error {

	g.mu.Lock()
	defer g.mu.Unlock()

	a := g.abstimmungIntern(abstimmungID)
	if a == nil {
		return fmt.Errorf("abstimmung %s gibt es nicht", abstimmungID)
	}
	a.zustand = kern.AbstimmungLaufend
	a.stimmberechtigt, a.anwesend, a.quorum = stimmberechtigt, anwesend, quorum
	return nil
}

// AbstimmungZustandSetzen schreibt den Übergang fest.
func (g *Gedaechtnis) AbstimmungZustandSetzen(ctx context.Context, abstimmungID string,
	zustand kern.Abstimmungszustand, zeit time.Time) error {

	g.mu.Lock()
	defer g.mu.Unlock()

	a := g.abstimmungIntern(abstimmungID)
	if a == nil {
		return fmt.Errorf("abstimmung %s gibt es nicht", abstimmungID)
	}
	a.zustand = zustand
	return nil
}

// StimmeAbgeben hält die Stimme fest — bei geheimer Wahl ohne die Person.
func (g *Gedaechtnis) StimmeAbgeben(ctx context.Context, abstimmungID, teilnahmeID string,
	wahl kern.Wahl, geheim bool) error {

	g.mu.Lock()
	defer g.mu.Unlock()

	a := g.abstimmungIntern(abstimmungID)
	if a == nil {
		return fmt.Errorf("abstimmung %s gibt es nicht", abstimmungID)
	}
	if a.abgegeben[teilnahmeID] {
		return fmt.Errorf("für diese teilnahme liegt bereits eine stimme vor")
	}
	a.abgegeben[teilnahmeID] = true

	if geheim {
		if a.zaehler == nil {
			a.zaehler = map[kern.Wahl]int{}
		}
		a.zaehler[wahl]++
		return nil
	}
	a.stimmen = append(a.stimmen, gStimme{teilnahmeID: teilnahmeID, wahl: wahl})
	return nil
}

// LetzteAbstimmung liefert die jüngste Abstimmung einer Sitzung.
func (g *Gedaechtnis) LetzteAbstimmung(ctx context.Context, sitzungID string) (*kern.Abstimmung, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var jüngste *gAbstimmung
	for _, a := range g.abstimmungen {
		if a.sitzungID == sitzungID && (jüngste == nil || a.folgeNr > jüngste.folgeNr) {
			jüngste = a
		}
	}
	if jüngste == nil {
		return nil, nil
	}

	platzVon := make(map[string]int, len(g.teilnahmen))
	for _, t := range g.teilnahmen {
		platzVon[t.id] = t.platz
	}

	a := &kern.Abstimmung{
		ID: jüngste.id, FolgeNr: jüngste.folgeNr, Titel: jüngste.titel,
		Art: jüngste.art, Zustand: jüngste.zustand,
		Stimmberechtigt: jüngste.stimmberechtigt, Anwesend: jüngste.anwesend, Quorum: jüngste.quorum,
		Abgegeben: map[int]bool{}, Namentlich: map[int]kern.Wahl{},
	}
	for teilnahmeID := range jüngste.abgegeben {
		a.Abgegeben[platzVon[teilnahmeID]] = true
	}
	a.Ja = jüngste.zaehler[kern.WahlJa]
	a.Nein = jüngste.zaehler[kern.WahlNein]
	a.Enthaltung = jüngste.zaehler[kern.WahlEnthaltung]
	for _, s := range jüngste.stimmen {
		switch s.wahl {
		case kern.WahlJa:
			a.Ja++
		case kern.WahlNein:
			a.Nein++
		case kern.WahlEnthaltung:
			a.Enthaltung++
		}
		if s.teilnahmeID != "" {
			a.Namentlich[platzVon[s.teilnahmeID]] = s.wahl
		}
	}
	return a, nil
}

// abstimmungIntern findet eine Abstimmung. Aufrufer hält g.mu.
func (g *Gedaechtnis) abstimmungIntern(id string) *gAbstimmung {
	for _, a := range g.abstimmungen {
		if a.id == id {
			return a
		}
	}
	return nil
}
