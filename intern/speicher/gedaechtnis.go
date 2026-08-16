package speicher

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tim-rose-hnvr/kameranachverfolgung/intern/kern"
)

// Ablage ist das, was der Server vom Speicher braucht. Postgres ist die
// Umsetzung im Betrieb, Gedaechtnis die für Tests ohne Datenbank.
type Ablage interface {
	SaalImportieren(ctx context.Context, d Saaldaten) (string, []kern.Platzaufbau, error)
	EreignisAnfuegen(ctx context.Context, saalID, art string, nutzlast map[string]any) (kern.Ereignis, error)
	Ereignisse(ctx context.Context, saalID string) ([]kern.Ereignis, error)
	Zaehlen(ctx context.Context, tabelle string) (int, error)
}

var (
	_ Ablage = (*Postgres)(nil)
	_ Ablage = (*Gedaechtnis)(nil)
)

// Gedaechtnis hält dieselben Daten im Arbeitsspeicher und setzt dieselben
// Eindeutigkeiten durch wie das SQL-Schema. Damit laufen die Tests ohne
// Datenbank; im Betrieb wird immer Postgres verwendet.
type Gedaechtnis struct {
	mu sync.Mutex

	organisationen map[string]string          // name -> id
	saele          map[string]string          // organisation_id|name -> id
	kameras        map[string]string          // saal_id|name -> id
	plaetze        map[string]string          // saal_id|nummer -> id
	presets        map[string]bool            // kamera_id|platz_id
	ketten         map[string][]kern.Ereignis // saal_id -> Kette
	naechsteID     int
}

// NeuGedaechtnis erzeugt eine leere Ablage im Arbeitsspeicher.
func NeuGedaechtnis() *Gedaechtnis {
	return &Gedaechtnis{
		organisationen: map[string]string{},
		saele:          map[string]string{},
		kameras:        map[string]string{},
		plaetze:        map[string]string{},
		presets:        map[string]bool{},
		ketten:         map[string][]kern.Ereignis{},
	}
}

func (g *Gedaechtnis) id() string {
	g.naechsteID++
	return fmt.Sprintf("id-%d", g.naechsteID)
}

// SaalImportieren legt fehlende Zeilen an und lässt vorhandene stehen.
func (g *Gedaechtnis) SaalImportieren(ctx context.Context, d Saaldaten) (string, []kern.Platzaufbau, error) {
	if err := d.Pruefen(); err != nil {
		return "", nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	organisationID, bekannt := g.organisationen[StandardOrganisation]
	if !bekannt {
		organisationID = g.id()
		g.organisationen[StandardOrganisation] = organisationID
	}

	saalSchluessel := organisationID + "|" + d.Saal
	saalID, bekannt := g.saele[saalSchluessel]
	if !bekannt {
		saalID = g.id()
		g.saele[saalSchluessel] = saalID
	}

	kameraIDs := make(map[string]string, len(d.Kameras))
	for _, k := range d.Kameras {
		schluessel := saalID + "|" + k.Name
		id, bekannt := g.kameras[schluessel]
		if !bekannt {
			id = g.id()
			g.kameras[schluessel] = id
		}
		kameraIDs[k.Name] = id
	}

	aufbau := make([]kern.Platzaufbau, 0, len(d.Plaetze))
	for _, p := range d.Plaetze {
		schluessel := fmt.Sprintf("%s|%d", saalID, p.Nummer)
		platzID, bekannt := g.plaetze[schluessel]
		if !bekannt {
			platzID = g.id()
			g.plaetze[schluessel] = platzID
		}
		g.presets[kameraIDs[p.Kamera]+"|"+platzID] = true

		var adresse string
		var kanal uint8
		for _, k := range d.Kameras {
			if k.Name == p.Kamera {
				adresse, kanal = k.Adresse, k.Kanal
				break
			}
		}
		aufbau = append(aufbau, kern.Platzaufbau{
			Nummer:        p.Nummer,
			Name:          p.Name,
			KameraName:    p.Kamera,
			KameraAdresse: adresse,
			Kanal:         kanal,
			Preset:        p.Preset,
		})
	}
	sort.Slice(aufbau, func(i, j int) bool { return aufbau[i].Nummer < aufbau[j].Nummer })
	return saalID, aufbau, nil
}

// EreignisAnfuegen verlängert die Kette des Saals.
func (g *Gedaechtnis) EreignisAnfuegen(ctx context.Context, saalID, art string, nutzlast map[string]any) (kern.Ereignis, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	kette := g.ketten[saalID]
	var vorher *kern.Ereignis
	if len(kette) > 0 {
		vorher = &kette[len(kette)-1]
	}
	e, err := kern.NaechstesEreignis(vorher, time.Now(), art, nutzlast)
	if err != nil {
		return kern.Ereignis{}, err
	}
	g.ketten[saalID] = append(kette, e)
	return e, nil
}

// Ereignisse liefert die Kette eines Saals.
func (g *Gedaechtnis) Ereignisse(ctx context.Context, saalID string) ([]kern.Ereignis, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]kern.Ereignis(nil), g.ketten[saalID]...), nil
}

// Zaehlen liefert die Zeilenzahl einer Tabelle.
func (g *Gedaechtnis) Zaehlen(ctx context.Context, tabelle string) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	switch tabelle {
	case "organisation":
		return len(g.organisationen), nil
	case "saal":
		return len(g.saele), nil
	case "kamera":
		return len(g.kameras), nil
	case "platz":
		return len(g.plaetze), nil
	case "preset":
		return len(g.presets), nil
	case "ereignis":
		anzahl := 0
		for _, kette := range g.ketten {
			anzahl += len(kette)
		}
		return anzahl, nil
	default:
		return 0, fmt.Errorf("unbekannte tabelle %q", tabelle)
	}
}
