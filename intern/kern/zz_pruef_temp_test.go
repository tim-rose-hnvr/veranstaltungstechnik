package kern_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
)

func saalBauen(t *testing.T, ablage speicher.Ablage, n int) (string, []kern.Platzaufbau) {
	t.Helper()
	saal := speicher.Saaldaten{
		Saal:    "Testraum",
		Kameras: []speicher.Kameradaten{{Name: "PTZ Mitte", Adresse: "192.168.1.50:52381", Kanal: 1}},
	}
	for i := 1; i <= n; i++ {
		saal.Plaetze = append(saal.Plaetze, speicher.Platzdaten{
			Nummer: i, Name: fmt.Sprintf("Platz %d", i), Kamera: "PTZ Mitte", Preset: uint8(i),
		})
	}
	saalID, plaetze, err := ablage.SaalImportieren(context.Background(), saal)
	if err != nil {
		t.Fatalf("saal: %v", err)
	}
	return saalID, plaetze
}

// Beleg 1: der Staffelstab wird saalweit aus der Kette gelesen und wandert in
// die naechste Sitzung — dort haengt er an einem Platz ohne Leitungsrolle.
func TestTempStaffelstabWandertInFolgesitzung(t *testing.T) {
	ctx := context.Background()
	ablage := speicher.NeuGedaechtnis()
	saalID, plaetze := saalBauen(t, ablage, 6)

	// Sitzung A: Platz 1 und Platz 2 sind zur Leitung berechtigt.
	standA, err := ablage.SitzungImportieren(ctx, saalID, speicher.Sitzungsdaten{
		Titel: "Sitzung A",
		Teilnahmen: []speicher.Teilnahmedaten{
			{Platz: 1, Person: "Anna", Rolle: "leitung", Pin: "1111"},
			{Platz: 2, Person: "Bert", Rolle: "leitung", Pin: "2222"},
			{Platz: 3, Person: "Cem", Rolle: "delegierter", Pin: "3333"},
		},
	}.MitVerzeichnis(""))
	if err != nil {
		t.Fatalf("sitzung A: %v", err)
	}
	kA, err := kern.Neu(kern.Aufbau{
		SaalID: saalID, SitzungID: standA.SitzungID, Titel: standA.Titel,
		SitzungZustand: standA.Zustand, Plaetze: plaetze, Teilnahmen: standA.Teilnahmen,
		MaxOffen: 3, Zeitlimit: 100 * time.Millisecond,
	}, &stilleKamera{}, ablage, nil)
	if err != nil {
		t.Fatalf("kern A: %v", err)
	}
	if err := kA.Anmelden(ctx, 1, "1111"); err != nil {
		t.Fatalf("anmelden 1: %v", err)
	}
	if err := kA.Anmelden(ctx, 2, "2222"); err != nil {
		t.Fatalf("anmelden 2: %v", err)
	}
	if err := kA.SitzungEroeffnen(ctx, 1); err != nil {
		t.Fatalf("eroeffnen: %v", err)
	}
	if err := kA.LeitungUebergeben(ctx, 1, 2); err != nil {
		t.Fatalf("uebergeben: %v", err)
	}
	if err := kA.SitzungSchliessen(ctx, 2); err != nil {
		t.Fatalf("schliessen: %v", err)
	}

	// Sitzung B im selben Saal: Platz 2 ist jetzt nur noch Delegierter,
	// die Leitung sitzt auf Platz 1.
	standB, err := ablage.SitzungImportieren(ctx, saalID, speicher.Sitzungsdaten{
		Titel: "Sitzung B",
		Teilnahmen: []speicher.Teilnahmedaten{
			{Platz: 1, Person: "Anna", Rolle: "leitung", Pin: "1111"},
			{Platz: 2, Person: "Bert", Rolle: "delegierter", Pin: "2222"},
			{Platz: 3, Person: "Cem", Rolle: "delegierter", Pin: "3333"},
		},
	}.MitVerzeichnis(""))
	if err != nil {
		t.Fatalf("sitzung B: %v", err)
	}
	leitung, err := ablage.LeitungAusKette(ctx, saalID)
	if err != nil {
		t.Fatalf("leitung: %v", err)
	}
	t.Logf("LeitungAusKette fuer Sitzung B liefert Platz %d (Zustand %s)", leitung, standB.Zustand)

	kB, err := kern.Neu(kern.Aufbau{
		SaalID: saalID, SitzungID: standB.SitzungID, Titel: standB.Titel,
		SitzungZustand: standB.Zustand, Plaetze: plaetze, Teilnahmen: standB.Teilnahmen,
		LeitungPlatz: leitung, MaxOffen: 3, Zeitlimit: 100 * time.Millisecond,
	}, &stilleKamera{}, ablage, nil)
	if err != nil {
		t.Fatalf("kern B: %v", err)
	}
	if err := kB.Anmelden(ctx, 1, "1111"); err != nil {
		t.Fatalf("anmelden B1: %v", err)
	}
	t.Logf("Zustand B: leitung_platz=%d", kB.Zustand().Sitzung.LeitungPlatz)
	t.Logf("Platz 1 eroeffnet: %v", kB.SitzungEroeffnen(ctx, 1))
	t.Logf("Platz 1 uebernimmt: %v", kB.LeitungUebernehmen(ctx, 1))
	t.Logf("Zustand nach Versuchen: %s", kB.Zustand().Sitzung.Zustand)
}

// Beleg 2: Uebernahme des Staffelstabs ist vor der Eroeffnung gesperrt, auch
// wenn der fuehrende Platz gar nicht besetzt ist.
func TestTempUebernahmeVorEroeffnung(t *testing.T) {
	ctx := context.Background()
	ablage := speicher.NeuGedaechtnis()
	saalID, plaetze := saalBauen(t, ablage, 6)

	stand, err := ablage.SitzungImportieren(ctx, saalID, speicher.Sitzungsdaten{
		Titel: "Sitzung C",
		Teilnahmen: []speicher.Teilnahmedaten{
			{Platz: 1, Person: "Anna", Rolle: "leitung", Pin: "1111"},
			{Platz: 5, Person: "Bert", Rolle: "leitung", Pin: "5555"},
		},
	}.MitVerzeichnis(""))
	if err != nil {
		t.Fatalf("sitzung: %v", err)
	}
	k, err := kern.Neu(kern.Aufbau{
		SaalID: saalID, SitzungID: stand.SitzungID, Titel: stand.Titel,
		SitzungZustand: stand.Zustand, Plaetze: plaetze, Teilnahmen: stand.Teilnahmen,
		MaxOffen: 3, Zeitlimit: 100 * time.Millisecond,
	}, &stilleKamera{}, ablage, nil)
	if err != nil {
		t.Fatalf("kern: %v", err)
	}
	// Platz 1 (Stellvertretung fehlt, Geraet defekt) meldet sich nie an.
	if err := k.Anmelden(ctx, 5, "5555"); err != nil {
		t.Fatalf("anmelden 5: %v", err)
	}
	t.Logf("fuehrender Platz laut Zustand: %d", k.Zustand().Sitzung.LeitungPlatz)
	t.Logf("Platz 5 uebernimmt: %v", k.LeitungUebernehmen(ctx, 5))
	t.Logf("Platz 5 eroeffnet:  %v", k.SitzungEroeffnen(ctx, 5))
	if ich := k.Ich(5); ich != nil {
		t.Logf("darf Platz 5: %v", ich.Darf)
	}
}
