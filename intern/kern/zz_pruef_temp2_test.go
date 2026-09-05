package kern_test

import (
	"context"
	"testing"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
)

// Beleg 3: zeigt der Staffelstab auf einen Platz ohne Leitungsrolle, waehrend
// die Sitzung laeuft, schaltet dieser Platz fremde Mikrofone ohne Worterteilung.
func TestTempDelegierterAufStaffelstab(t *testing.T) {
	ctx := context.Background()
	ablage := speicher.NeuGedaechtnis()
	saalID, plaetze := saalBauen(t, ablage, 6)

	stand, err := ablage.SitzungImportieren(ctx, saalID, speicher.Sitzungsdaten{
		Titel: "Sitzung D",
		Teilnahmen: []speicher.Teilnahmedaten{
			{Platz: 1, Person: "Anna", Rolle: "leitung", Pin: "1111"},
			{Platz: 2, Person: "Bert", Rolle: "delegierter", Pin: "2222"},
			{Platz: 3, Person: "Cem", Rolle: "delegierter", Pin: "3333"},
		},
	}.MitVerzeichnis(""))
	if err != nil {
		t.Fatalf("sitzung: %v", err)
	}
	k, err := kern.Neu(kern.Aufbau{
		SaalID: saalID, SitzungID: stand.SitzungID, Titel: stand.Titel,
		SitzungZustand: kern.SitzungLaufend, // aus der Datenbank: Sitzung lief schon
		Plaetze:        plaetze, Teilnahmen: stand.Teilnahmen,
		LeitungPlatz: 2, // aus der Kette, Platz 2 ist aber nur Delegierter
		MaxOffen:     3, Zeitlimit: 100 * time.Millisecond,
	}, &stilleKamera{}, ablage, nil)
	if err != nil {
		t.Fatalf("kern: %v", err)
	}
	for _, a := range []struct {
		nr  int
		pin string
	}{{1, "1111"}, {2, "2222"}, {3, "3333"}} {
		if err := k.Anmelden(ctx, a.nr, a.pin); err != nil {
			t.Fatalf("anmelden %d: %v", a.nr, err)
		}
	}
	t.Logf("Platz 2 (Delegierter) oeffnet fremdes Mikro auf Platz 3: %v", k.MikroAn(ctx, 2, 3))
	t.Logf("Platz 3 Mikro offen: %v", k.Zustand().Plaetze[2].Mikro)
	t.Logf("Platz 2 entzieht Platz 3 wieder das Mikro: %v", k.MikroAus(ctx, 2, 3))
	t.Logf("echte Leitung Platz 1 erteilt Wort an 3: %v", k.WortErteilen(ctx, 1, 3))
	if ich := k.Ich(1); ich != nil {
		t.Logf("darf die echte Leitung (Platz 1): %v", ich.Darf)
	}
	if ich := k.Ich(2); ich != nil {
		t.Logf("darf Platz 2 (Delegierter mit Staffelstab): %v", ich.Darf)
	}
	k.KameraAbwarten()
}
