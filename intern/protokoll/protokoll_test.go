package protokoll_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/protokoll"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
)

type stilleKamera struct{}

func (stilleKamera) PresetAbrufen(ctx context.Context, adresse string, kanal, preset uint8) error {
	return nil
}

// sitzungFahren spielt eine vollständige Sitzung durch und liefert das
// Protokoll dazu.
func sitzungFahren(t *testing.T, art kern.Abstimmungsart) string {
	t.Helper()
	ctx := context.Background()
	ablage := speicher.NeuGedaechtnis()

	saalID, plaetze, err := ablage.SaalImportieren(ctx, speicher.Saaldaten{
		Saal:    "Testraum",
		Kameras: []speicher.Kameradaten{{Name: "PTZ Mitte", Adresse: "127.0.0.1:52381", Kanal: 1}},
		Plaetze: []speicher.Platzdaten{
			{Nummer: 1, Name: "Vorsitz", Kamera: "PTZ Mitte", Preset: 1},
			{Nummer: 2, Name: "Platz 2", Kamera: "PTZ Mitte", Preset: 2},
		},
	})
	if err != nil {
		t.Fatalf("saal: %v", err)
	}
	stand, err := ablage.SitzungImportieren(ctx, saalID, speicher.Sitzungsdaten{
		Titel: "Vorstandssitzung",
		Teilnahmen: []speicher.Teilnahmedaten{
			{Platz: 1, Person: "Anke Bergmann", Rolle: "leitung", Pin: "1111"},
			{Platz: 2, Person: "Jonas Öztürk", Rolle: "delegierter", Pin: "2222"},
		},
	})
	if err != nil {
		t.Fatalf("sitzung: %v", err)
	}

	k, err := kern.Neu(kern.Aufbau{
		SaalID: saalID, SitzungID: stand.SitzungID, Titel: stand.Titel,
		SitzungZustand: stand.Zustand, Plaetze: plaetze, Teilnahmen: stand.Teilnahmen,
		MaxOffen: 2, Zeitlimit: 50 * time.Millisecond,
	}, stilleKamera{}, ablage, nil)
	if err != nil {
		t.Fatalf("kern: %v", err)
	}

	mussKlappen := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("schritt fehlgeschlagen: %v", err)
		}
	}
	mussKlappen(k.Anmelden(ctx, 1, "1111"))
	mussKlappen(k.Anmelden(ctx, 2, "2222"))
	mussKlappen(k.SitzungEroeffnen(ctx, 1))
	mussKlappen(k.WortMelden(ctx, 2))
	mussKlappen(k.WortErteilen(ctx, 1, 2))
	mussKlappen(k.AbstimmungStarten(ctx, 1, "Haushalt 2027", art))
	mussKlappen(k.StimmeAbgeben(ctx, 1, kern.WahlJa, "", ""))
	mussKlappen(k.StimmeAbgeben(ctx, 2, kern.WahlNein, "", ""))
	mussKlappen(k.AbstimmungBeenden(ctx, 1))
	mussKlappen(k.AbstimmungFeststellen(ctx, 1))
	mussKlappen(k.SitzungSchliessen(ctx, 1))
	k.KameraAbwarten()

	schreiber := protokoll.Neu(saalID, "Testraum", ablage, stand.Teilnahmen)
	text, err := schreiber.Markdown(ctx, stand.SitzungID, stand.Titel)
	if err != nil {
		t.Fatalf("protokoll: %v", err)
	}
	return text
}

// TestProtokollEnthaeltDenVerlauf: was passiert ist, steht drin — mit Namen
// statt Platznummern.
func TestProtokollEnthaeltDenVerlauf(t *testing.T) {
	text := sitzungFahren(t, kern.AbstimmungNamentlich)

	for _, muss := range []string{
		"# Protokoll — Vorstandssitzung",
		"Kette nachgerechnet: ja",
		"Die Sitzung wird durch Anke Bergmann eröffnet.",
		"Jonas Öztürk meldet sich zu Wort.",
		"Jonas Öztürk erhält das Wort.",
		"Abstimmung „Haushalt 2027\" (namentlich) wird eröffnet.",
		"Quorum 2",
		"ausgezählt: 1 Ja, 1 Nein, 0 Enthaltungen",
		"## Beschlüsse",
		"1 Ja, 1 Nein, 0 Enthaltungen — abgelehnt.",
		"Die Sitzung wird durch Anke Bergmann geschlossen.",
	} {
		if !strings.Contains(text, muss) {
			t.Errorf("im protokoll fehlt: %q\n\n%s", muss, text)
		}
	}

	// Vor der Eröffnung gibt es keine Zeitachse — das steht auch so da.
	if !strings.Contains(text, "| vorab | Anke Bergmann nimmt auf Platz 1 Platz. |") {
		t.Errorf("die anmeldung vor der eröffnung fehlt oder trägt eine falsche zeit:\n%s", text)
	}
	// Einzelne Stimmen gehören nicht in den Verlauf.
	if strings.Contains(text, "stimme_abgegeben") {
		t.Error("rohe ereignisnamen haben im protokoll nichts verloren")
	}
}

// TestProtokollVerraetGeheimeWahlNicht: bei geheimer Wahl steht das Ergebnis
// im Protokoll, nie aber, wer wie gestimmt hat.
func TestProtokollVerraetGeheimeWahlNicht(t *testing.T) {
	text := sitzungFahren(t, kern.AbstimmungGeheim)

	if !strings.Contains(text, "ausgezählt: 1 Ja, 1 Nein") {
		t.Errorf("das ergebnis fehlt:\n%s", text)
	}
	for _, darfNicht := range []string{
		"Anke Bergmann stimmt", "Jonas Öztürk stimmt",
		"Anke Bergmann: ja", "Jonas Öztürk: nein",
	} {
		if strings.Contains(text, darfNicht) {
			t.Errorf("die geheime wahl ist verraten: %q\n\n%s", darfNicht, text)
		}
	}
}
