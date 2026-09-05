package speicher_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
)

// geheimeWahlAufbauen legt eine geheime Wahl an und lässt vier verschieden
// stimmen. Zurück kommt die Kennung der Abstimmung und die Wahrheit, gegen
// die geprüft wird.
func geheimeWahlAufbauen(t *testing.T, ablage speicher.Ablage) (saalID, sitzungID, abstimmungID string, wahrheit map[int]kern.Wahl) {
	t.Helper()
	ctx := context.Background()

	saalID, _, err := ablage.SaalImportieren(ctx, testsaal())
	if err != nil {
		t.Fatalf("saal einlesen: %v", err)
	}
	stand, err := ablage.SitzungImportieren(ctx, saalID, testsitzung())
	if err != nil {
		t.Fatalf("sitzung einlesen: %v", err)
	}
	sitzungID = stand.SitzungID

	id, _, err := ablage.AbstimmungAnlegen(ctx, stand.SitzungID, "Wahl der Vorsitzenden", kern.AbstimmungGeheim)
	if err != nil {
		t.Fatalf("abstimmung anlegen: %v", err)
	}

	// Bewusst verschieden, damit eine Aufdeckung auch etwas zu verraten hätte.
	wahlen := []kern.Wahl{kern.WahlJa, kern.WahlNein, kern.WahlEnthaltung}
	wahrheit = map[int]kern.Wahl{}
	for i, teilnahme := range stand.Teilnahmen {
		wahl := wahlen[i%len(wahlen)]
		if err := ablage.StimmeAbgeben(ctx, id, teilnahme.ID, wahl, true); err != nil {
			t.Fatalf("stimme für platz %d: %v", teilnahme.PlatzNummer, err)
		}
		wahrheit[teilnahme.PlatzNummer] = wahl
	}
	return saalID, sitzungID, id, wahrheit
}

// TestGeheimeWahlZaehltRichtig: die Zählung muss stimmen, obwohl keine
// einzelne Stimme gespeichert wird — sonst wäre die Vertraulichkeit mit
// Unbrauchbarkeit erkauft. Und beim Neustart kommt sie unverändert zurück.
func TestGeheimeWahlZaehltRichtig(t *testing.T) {
	for name, bauen := range ablagen(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ablage := bauen(t)
			_, sitzungID, _, wahrheit := geheimeWahlAufbauen(t, ablage)

			erwartet := map[kern.Wahl]int{}
			for _, w := range wahrheit {
				erwartet[w]++
			}

			// So, wie der Server nach einem Absturz liest.
			a, err := ablage.LetzteAbstimmung(ctx, sitzungID)
			if err != nil {
				t.Fatalf("abstimmung laden: %v", err)
			}
			if a == nil {
				t.Fatal("die abstimmung muss den neustart überleben")
			}
			if a.Ja != erwartet[kern.WahlJa] || a.Nein != erwartet[kern.WahlNein] ||
				a.Enthaltung != erwartet[kern.WahlEnthaltung] {
				t.Errorf("zählung falsch: ja=%d nein=%d enthaltung=%d, erwartet ja=%d nein=%d enthaltung=%d",
					a.Ja, a.Nein, a.Enthaltung,
					erwartet[kern.WahlJa], erwartet[kern.WahlNein], erwartet[kern.WahlEnthaltung])
			}

			// Wer abgestimmt hat, muss bekannt bleiben — sonst ließe sich
			// doppelte Stimmabgabe nicht verhindern.
			if len(a.Abgegeben) != len(wahrheit) {
				t.Errorf("%d stimmabgaben erwartet, %d bekommen", len(wahrheit), len(a.Abgegeben))
			}
			// Wie abgestimmt wurde, darf nirgends je Platz stehen.
			if len(a.Namentlich) != 0 {
				t.Errorf("bei geheimer wahl darf es keine namentliche zuordnung geben: %v", a.Namentlich)
			}
		})
	}
}

// TestGeheimeWahlUeberlebtDenAngriff führt den Angriff aus, der die geheime
// Wahl einmal vollständig aufgedeckt hat: stimmabgabe (WER) und stimme (WAS)
// entstanden in derselben Transaktion und trugen deshalb dasselbe xmin — ein
// einziges JOIN genügte. Der Test bleibt stehen, damit es nicht zurückkommt.
//
// Läuft nur gegen echtes PostgreSQL; xmin gibt es nur dort.
func TestGeheimeWahlUeberlebtDenAngriff(t *testing.T) {
	dsn := os.Getenv("SITZUNG_TEST_DB")
	if dsn == "" {
		t.Skip("ohne SITZUNG_TEST_DB nicht prüfbar — xmin gibt es nur in PostgreSQL")
	}
	ctx := context.Background()
	ablage := frischePostgres(t, dsn)
	saalID, _, id, wahrheit := geheimeWahlAufbauen(t, ablage)

	teich, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("prüfverbindung: %v", err)
	}
	defer teich.Close()

	// Angriff 1: die Transaktionsnummer verbindet WER und WAS.
	var ueberXmin int
	if err := teich.QueryRow(ctx, `
		SELECT count(*) FROM stimmabgabe a
		JOIN stimme s ON a.xmin = s.xmin AND a.abstimmung_id = s.abstimmung_id
		WHERE a.abstimmung_id = $1`, id).Scan(&ueberXmin); err != nil {
		t.Fatalf("angriff über xmin: %v", err)
	}
	if ueberXmin != 0 {
		t.Errorf("die geheime wahl ist über xmin aufdeckbar: %d von %d stimmen zugeordnet",
			ueberXmin, len(wahrheit))
	}

	// Angriff 2: es darf überhaupt keine Einzelstimme geben, die man über
	// Reihenfolge, Einfügeposition oder Zeit zuordnen könnte.
	var einzelstimmen int
	if err := teich.QueryRow(ctx,
		"SELECT count(*) FROM stimme WHERE abstimmung_id = $1", id).Scan(&einzelstimmen); err != nil {
		t.Fatalf("einzelstimmen zählen: %v", err)
	}
	if einzelstimmen != 0 {
		t.Errorf("bei geheimer wahl darf keine einzelstimme gespeichert sein, es sind %d", einzelstimmen)
	}

	// Angriff 3: die Zähler dürfen nicht verraten, welche Wahl die letzte
	// Stimmabgabe erhöht hat — alle drei müssen dasselbe xmin tragen.
	var verschiedeneXmin int
	if err := teich.QueryRow(ctx,
		// xid kennt keine Ordnung, deshalb über den Text zählen.
		"SELECT count(DISTINCT xmin::text) FROM stimme_zaehler WHERE abstimmung_id = $1", id).Scan(&verschiedeneXmin); err != nil {
		t.Fatalf("zähler prüfen: %v", err)
	}
	if verschiedeneXmin != 1 {
		t.Errorf("die drei zähler tragen %d verschiedene transaktionsnummern — "+
			"damit verrät xmin, welche wahl die letzte stimme erhöht hat", verschiedeneXmin)
	}

	// Gegenprobe: bei OFFENER Wahl ist die Zuordnung gewollt und muss da sein.
	// Sonst würde dieser Test auch dann grün, wenn gar nichts mehr gespeichert wird.
	stand, err := ablage.SitzungImportieren(ctx, saalID, testsitzung())
	if err != nil {
		t.Fatalf("sitzung: %v", err)
	}
	offen, _, err := ablage.AbstimmungAnlegen(ctx, stand.SitzungID, "Offener Beschluss", kern.AbstimmungOffen)
	if err != nil {
		t.Fatalf("offene abstimmung: %v", err)
	}
	if err := ablage.StimmeAbgeben(ctx, offen, stand.Teilnahmen[0].ID, kern.WahlJa, false); err != nil {
		t.Fatalf("offene stimme: %v", err)
	}
	var offeneZuordnung int
	if err := teich.QueryRow(ctx,
		"SELECT count(*) FROM stimme WHERE abstimmung_id = $1 AND teilnahme_id IS NOT NULL",
		offen).Scan(&offeneZuordnung); err != nil {
		t.Fatalf("offene zuordnung: %v", err)
	}
	if offeneZuordnung != 1 {
		t.Errorf("bei offener wahl gehört die zuordnung dazu, gefunden: %d", offeneZuordnung)
	}
}
