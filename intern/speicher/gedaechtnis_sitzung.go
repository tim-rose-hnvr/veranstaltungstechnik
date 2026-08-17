package speicher

import (
	"context"
	"fmt"
	"sort"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
)

type gSitzung struct {
	id      string
	saalID  string
	titel   string
	zustand kern.Sitzungszustand
	beginn  *time.Time
}

type gTeilnahme struct {
	id          string
	personID    string
	platz       int
	rolle       kern.Rolle
	zustand     kern.Teilnahmezustand
	pinHash     []byte
	widerspruch bool
}

type gTop struct {
	id          string
	sitzungID   string
	nummer      int
	titel       string
	oeffentlich bool
	zustand     kern.Topzustand
}

type gWortmeldung struct {
	id          string
	sitzungID   string
	teilnahmeID string
	folgeNr     int64
	platz       int
	zustand     kern.Wortzustand
}

// SitzungImportieren legt fehlende Zeilen an und lässt vorhandene stehen.
func (g *Gedaechtnis) SitzungImportieren(ctx context.Context, saalID string, d Sitzungsdaten) (Sitzungsstand, error) {
	if err := d.Pruefen(); err != nil {
		return Sitzungsstand{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	organisationID := g.organisationen[StandardOrganisation]

	var sitzung *gSitzung
	for _, s := range g.sitzungen {
		if s.saalID == saalID && s.titel == d.Titel {
			sitzung = s
			break
		}
	}
	if sitzung == nil {
		sitzung = &gSitzung{id: g.id(), saalID: saalID, titel: d.Titel, zustand: kern.SitzungVorbereitet}
		g.sitzungen[sitzung.id] = sitzung
	}

	stand := Sitzungsstand{SitzungID: sitzung.id, Titel: sitzung.titel, Zustand: sitzung.zustand, Beginn: sitzung.beginn}

	for _, t := range d.Teilnahmen {
		if _, bekannt := g.plaetze[fmt.Sprintf("%s|%d", saalID, t.Platz)]; !bekannt {
			return Sitzungsstand{}, fmt.Errorf("platz %d gibt es im saal nicht", t.Platz)
		}

		personSchluessel := organisationID + "|" + t.Person
		personID, bekannt := g.personen[personSchluessel]
		if !bekannt {
			personID = g.id()
			g.personen[personSchluessel] = personID
		}

		schluessel := fmt.Sprintf("%s|%d", sitzung.id, t.Platz)
		teilnahme, bekannt := g.teilnahmen[schluessel]
		if !bekannt {
			hash, err := bcrypt.GenerateFromPassword([]byte(t.Pin), bcrypt.MinCost)
			if err != nil {
				return Sitzungsstand{}, err
			}
			teilnahme = &gTeilnahme{
				id: g.id(), personID: personID, platz: t.Platz,
				zustand: kern.TeilnahmeEingeladen, pinHash: hash,
			}
			g.teilnahmen[schluessel] = teilnahme
		} else if bcrypt.CompareHashAndPassword(teilnahme.pinHash, []byte(t.Pin)) != nil {
			hash, err := bcrypt.GenerateFromPassword([]byte(t.Pin), bcrypt.MinCost)
			if err != nil {
				return Sitzungsstand{}, err
			}
			teilnahme.pinHash = hash
		}
		teilnahme.personID = personID
		teilnahme.rolle = kern.Rolle(t.Rolle)
		teilnahme.widerspruch = t.Widerspruch

		stand.Teilnahmen = append(stand.Teilnahmen, kern.Teilnahmeaufbau{
			ID:          teilnahme.id,
			PlatzNummer: t.Platz,
			Person:      t.Person,
			Rolle:       teilnahme.rolle,
			PinHash:     teilnahme.pinHash,
			Widerspruch: teilnahme.widerspruch,
		})
	}
	sort.Slice(stand.Teilnahmen, func(i, j int) bool {
		return stand.Teilnahmen[i].PlatzNummer < stand.Teilnahmen[j].PlatzNummer
	})

	// Die Tagesordnung wird abgeglichen: der Zustand eines schon aufgerufenen
	// Punktes bleibt stehen, Titel und Öffentlichkeit kommen aus der Datei.
	for _, t := range d.Tagesordnung {
		schluessel := fmt.Sprintf("%s|%d", sitzung.id, t.Nummer)
		top, bekannt := g.tagesordnung[schluessel]
		if !bekannt {
			top = &gTop{
				id: g.id(), sitzungID: sitzung.id, nummer: t.Nummer, zustand: kern.TopOffen,
			}
			g.tagesordnung[schluessel] = top
		}
		top.titel = t.Titel
		top.oeffentlich = t.IstOeffentlich()

		stand.Tagesordnung = append(stand.Tagesordnung, kern.Tagesordnungspunkt{
			ID: top.id, Nummer: top.nummer, Titel: top.titel,
			Oeffentlich: top.oeffentlich, Zustand: top.zustand,
		})

		unterlagen, err := unterlagenLesen(d.Verzeichnis(), t)
		if err != nil {
			return Sitzungsstand{}, err
		}
		for _, u := range unterlagen {
			schluessel := sitzung.id + "|" + u.Datei
			id, bekannt := g.unterlagen[schluessel]
			if !bekannt {
				id = g.id()
				g.unterlagen[schluessel] = id
			}
			u.ID = id
			stand.Unterlagen = append(stand.Unterlagen, u)
		}
	}
	sort.Slice(stand.Tagesordnung, func(i, j int) bool {
		return stand.Tagesordnung[i].Nummer < stand.Tagesordnung[j].Nummer
	})

	for _, w := range g.wortmeldungen {
		if w.sitzungID == sitzung.id && w.zustand.Offen() {
			stand.Wortmeldungen = append(stand.Wortmeldungen, kern.Wortmeldung{
				ID: w.id, FolgeNr: w.folgeNr, PlatzNummer: w.platz, Zustand: w.zustand,
			})
		}
	}
	return stand, nil
}

// SitzungZustandSetzen hält den Zustandswechsel fest.
func (g *Gedaechtnis) SitzungZustandSetzen(ctx context.Context, sitzungID string,
	zustand kern.Sitzungszustand, zeit time.Time) error {

	g.mu.Lock()
	defer g.mu.Unlock()

	s, bekannt := g.sitzungen[sitzungID]
	if !bekannt {
		return fmt.Errorf("sitzung %s gibt es nicht", sitzungID)
	}
	s.zustand = zustand
	if zustand == kern.SitzungLaufend && s.beginn == nil {
		s.beginn = &zeit
	}
	return nil
}

// TeilnahmeZustandSetzen hält Kommen und Gehen fest.
func (g *Gedaechtnis) TeilnahmeZustandSetzen(ctx context.Context, teilnahmeID string, zustand kern.Teilnahmezustand) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, t := range g.teilnahmen {
		if t.id == teilnahmeID {
			t.zustand = zustand
			return nil
		}
	}
	return fmt.Errorf("teilnahme %s gibt es nicht", teilnahmeID)
}

// WortmeldungAnlegen hängt eine Meldung an die Redeliste.
func (g *Gedaechtnis) WortmeldungAnlegen(ctx context.Context, sitzungID, teilnahmeID string) (string, int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var platz int
	gefunden := false
	for _, t := range g.teilnahmen {
		if t.id == teilnahmeID {
			platz, gefunden = t.platz, true
			break
		}
	}
	if !gefunden {
		return "", 0, fmt.Errorf("teilnahme %s gibt es nicht", teilnahmeID)
	}

	var hoechste int64
	for _, w := range g.wortmeldungen {
		if w.sitzungID == sitzungID && w.folgeNr > hoechste {
			hoechste = w.folgeNr
		}
	}
	w := &gWortmeldung{
		id: g.id(), sitzungID: sitzungID, teilnahmeID: teilnahmeID,
		folgeNr: hoechste + 1, platz: platz, zustand: kern.WortGemeldet,
	}
	g.wortmeldungen = append(g.wortmeldungen, w)
	return w.id, w.folgeNr, nil
}

// WortmeldungZustandSetzen schreibt den Übergang fest.
func (g *Gedaechtnis) WortmeldungZustandSetzen(ctx context.Context, wortmeldungID string, zustand kern.Wortzustand) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, w := range g.wortmeldungen {
		if w.id == wortmeldungID {
			w.zustand = zustand
			return nil
		}
	}
	return fmt.Errorf("wortmeldung %s gibt es nicht", wortmeldungID)
}

// OffeneWortmeldungen liefert die Redeliste einer Sitzung.
func (g *Gedaechtnis) OffeneWortmeldungen(ctx context.Context, sitzungID string) ([]kern.Wortmeldung, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var liste []kern.Wortmeldung
	for _, w := range g.wortmeldungen {
		if w.sitzungID == sitzungID && w.zustand.Offen() {
			liste = append(liste, kern.Wortmeldung{
				ID: w.id, FolgeNr: w.folgeNr, PlatzNummer: w.platz, Zustand: w.zustand,
			})
		}
	}
	sort.Slice(liste, func(i, j int) bool { return liste[i].FolgeNr < liste[j].FolgeNr })
	return liste, nil
}

// TopZustandSetzen schreibt den Übergang eines Tagesordnungspunkts fest.
func (g *Gedaechtnis) TopZustandSetzen(ctx context.Context, topID string, zustand kern.Topzustand) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, t := range g.tagesordnung {
		if t.id == topID {
			t.zustand = zustand
			return nil
		}
	}
	return fmt.Errorf("tagesordnungspunkt %s gibt es nicht", topID)
}

// LeitungAusKette liest den Staffelstab aus der Kette. Übergabe und Übernahme
// zählen gleich — sonst geht die übernommene Leitung beim Neustart verloren.
func (g *Gedaechtnis) LeitungAusKette(ctx context.Context, saalID string) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	platz := 0
	for _, e := range g.ketten[saalID] {
		if e.Art != "leitung_uebergeben" && e.Art != "leitung_uebernommen" {
			continue
		}
		if an, gefunden := e.Nutzlast["an"]; gefunden {
			switch wert := an.(type) {
			case int:
				platz = wert
			case float64:
				platz = int(wert)
			}
		}
	}
	return platz, nil
}
