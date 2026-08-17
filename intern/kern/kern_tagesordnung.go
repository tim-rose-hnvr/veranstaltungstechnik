package kern

import (
	"context"
	"fmt"
)

// Tagesordnungsablage hält die Zustände der Tagesordnung fest.
type Tagesordnungsablage interface {
	TopZustandSetzen(ctx context.Context, topID string, zustand Topzustand) error
}

// TopAufrufen ruft einen Tagesordnungspunkt auf. Ein laufender Punkt wird
// dabei abgeschlossen — es ist immer höchstens einer in Arbeit.
func (k *Kern) TopAufrufen(ctx context.Context, absender, nummer int) error {
	k.mu.Lock()
	if err := k.darfPlatz(absender, AktionTopAufrufen); err != nil {
		k.mu.Unlock()
		return err
	}
	ziel := k.topIntern(nummer)
	if ziel == nil {
		k.mu.Unlock()
		return fehler(CodeTopUnbekannt, fmt.Sprintf("Tagesordnungspunkt %d gibt es nicht", nummer))
	}
	if ziel.Zustand == TopLaufend {
		k.mu.Unlock()
		return nil
	}
	if !TopUebergangErlaubt(ziel.Zustand, TopLaufend) {
		k.mu.Unlock()
		return fehler(CodeTopGeschlossen,
			fmt.Sprintf("Tagesordnungspunkt %d ist %s und wird nicht mehr aufgerufen", nummer, ziel.Zustand))
	}

	vorher := k.laufenderTopIntern()
	if vorher != nil {
		if err := k.topSetzenIntern(ctx, vorher, TopAbgeschlossen, absender); err != nil {
			k.mu.Unlock()
			return err
		}
	}
	if err := k.topSetzenIntern(ctx, ziel, TopLaufend, absender); err != nil {
		k.mu.Unlock()
		return err
	}
	k.aufzeichnungPruefenIntern(ctx)
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// TopAbschliessen beendet den laufenden Punkt, ohne den nächsten aufzurufen.
func (k *Kern) TopAbschliessen(ctx context.Context, absender int) error {
	return k.topBeenden(ctx, absender, AktionTopAbschliessen, TopAbgeschlossen)
}

// TopVertagen schiebt den laufenden Punkt in eine andere Sitzung. Er wird in
// dieser Sitzung nicht mehr aufgerufen.
func (k *Kern) TopVertagen(ctx context.Context, absender int) error {
	return k.topBeenden(ctx, absender, AktionTopVertagen, TopVertagt)
}

func (k *Kern) topBeenden(ctx context.Context, absender int, aktion string, neu Topzustand) error {
	k.mu.Lock()
	if err := k.darfPlatz(absender, aktion); err != nil {
		k.mu.Unlock()
		return err
	}
	laufend := k.laufenderTopIntern()
	if laufend == nil {
		k.mu.Unlock()
		return fehler(CodeKeinTop, "Es ist kein Tagesordnungspunkt aufgerufen")
	}
	if err := k.topSetzenIntern(ctx, laufend, neu, absender); err != nil {
		k.mu.Unlock()
		return err
	}
	k.aufzeichnungPruefenIntern(ctx)
	k.stand++
	k.mu.Unlock()

	k.melder()
	return nil
}

// topSetzenIntern schreibt Ereignis und Zustand eines Punktes.
// Aufrufer hält k.mu.
func (k *Kern) topSetzenIntern(ctx context.Context, t *Tagesordnungspunkt,
	neu Topzustand, absender int) *Fehler {

	art := "top_aufgerufen"
	switch neu {
	case TopAbgeschlossen:
		art = "top_abgeschlossen"
	case TopVertagt:
		art = "top_vertagt"
	}
	if err := k.schreiben(ctx, art, map[string]any{
		"top": t.Nummer, "titel": t.Titel, "oeffentlich": t.Oeffentlich, "von": absender,
	}); err != nil {
		return err
	}
	t.Zustand = neu
	if k.ablage != nil && t.ID != "" {
		if err := k.ablage.TopZustandSetzen(ctx, t.ID, neu); err != nil {
			k.protokoll.Error("tagesordnungspunkt nicht gespeichert", "top", t.Nummer, "grund", err)
		}
	}
	return nil
}

// aufzeichnungPruefenIntern gleicht Stream und Aufzeichnung mit der
// Tagesordnung ab. Bei einem nicht öffentlichen Punkt pausieren beide — das
// entscheidet der Sitzungszustand, nicht die Technik und nicht ein Handgriff.
// Aufrufer hält k.mu.
func (k *Kern) aufzeichnungPruefenIntern(ctx context.Context) {
	soll := k.aufzeichnungSollIntern()
	if soll == k.aufzeichnung {
		return
	}

	art := "aufzeichnung_pausiert"
	grund := "nicht öffentlicher Tagesordnungspunkt"
	if soll {
		art = "aufzeichnung_fortgesetzt"
		grund = "öffentlicher Tagesordnungspunkt"
	}
	nutzlast := map[string]any{"grund": grund}
	if t := k.laufenderTopIntern(); t != nil {
		nutzlast["top"] = t.Nummer
	}
	if err := k.schreiben(ctx, art, nutzlast); err != nil {
		k.protokoll.Error("aufzeichnungswechsel nicht protokolliert", "art", art)
		return
	}
	k.aufzeichnung = soll
}

// aufzeichnungSollIntern sagt, ob gerade aufgezeichnet und gestreamt werden
// darf. Aufrufer hält k.mu.
func (k *Kern) aufzeichnungSollIntern() bool {
	if k.sitzung != SitzungLaufend {
		return false
	}
	if t := k.laufenderTopIntern(); t != nil {
		return t.Oeffentlich
	}
	// Ohne aufgerufenen Punkt läuft die Sitzung öffentlich — eine
	// Tagesordnung, die keiner aufruft, verschweigt nichts.
	return true
}

// laufenderTopIntern liefert den aufgerufenen Punkt oder nil.
// Aufrufer hält k.mu.
func (k *Kern) laufenderTopIntern() *Tagesordnungspunkt {
	for _, t := range k.tagesordnung {
		if t.Zustand == TopLaufend {
			return t
		}
	}
	return nil
}

// topIntern findet einen Punkt nach Nummer. Aufrufer hält k.mu.
func (k *Kern) topIntern(nummer int) *Tagesordnungspunkt {
	for _, t := range k.tagesordnung {
		if t.Nummer == nummer {
			return t
		}
	}
	return nil
}

// tagesordnungZustandIntern baut die Sicht des Clients. Aufrufer hält k.mu.
func (k *Kern) tagesordnungZustandIntern() []TopZustand {
	liste := make([]TopZustand, 0, len(k.tagesordnung))
	for _, t := range k.tagesordnung {
		liste = append(liste, TopZustand{
			Nummer: t.Nummer, Titel: t.Titel, Oeffentlich: t.Oeffentlich, Zustand: t.Zustand,
		})
	}
	return liste
}
