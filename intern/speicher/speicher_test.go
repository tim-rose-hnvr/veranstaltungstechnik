package speicher_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
)

// Die Tests laufen gegen die Ablage im Arbeitsspeicher und, wenn
// SITZUNG_TEST_DB gesetzt ist, zusätzlich gegen eine echte PostgreSQL.
// Beispiel:
//
//	SITZUNG_TEST_DB=postgres://sitzung:geheim@localhost:5432/sitzung_test?sslmode=disable go test ./...
func ablagen(t *testing.T) map[string]func(*testing.T) speicher.Ablage {
	t.Helper()
	bauer := map[string]func(*testing.T) speicher.Ablage{
		"gedaechtnis": func(*testing.T) speicher.Ablage { return speicher.NeuGedaechtnis() },
	}
	if dsn := os.Getenv("SITZUNG_TEST_DB"); dsn != "" {
		bauer["postgres"] = func(t *testing.T) speicher.Ablage { return frischePostgres(t, dsn) }
	}
	return bauer
}

func frischePostgres(t *testing.T, dsn string) speicher.Ablage {
	t.Helper()
	ctx := context.Background()

	teich, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("testdatenbank: %v", err)
	}
	defer teich.Close()
	// Das ganze Schema wegwerfen statt einer Liste von Tabellen: eine
	// Aufzählung von Hand driftet von den Migrationen weg, und dann läuft der
	// Test gegen ein halb altes Schema, ohne dass etwas auffällt.
	if _, err := teich.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatalf("testdatenbank leeren: %v", err)
	}

	p, err := speicher.Verbinden(ctx, dsn)
	if err != nil {
		t.Fatalf("verbinden: %v", err)
	}
	t.Cleanup(p.Schliessen)
	// Dieselbe Liste wie im Betrieb — eine eigene Aufzählung im Test hatte
	// schon einmal dazu geführt, dass die Tests gegen ein älteres Schema liefen.
	for _, m := range speicher.Migrationen {
		roh, err := os.ReadFile(filepath.Join("../../migrationen", m.Datei))
		if err != nil {
			t.Fatalf("migration lesen: %v", err)
		}
		if err := p.SchemaSicherstellen(ctx, m.Waechter, string(roh)); err != nil {
			t.Fatalf("schema anlegen: %v", err)
		}
	}
	return p
}

func testsaal() speicher.Saaldaten {
	return speicher.Saaldaten{
		Saal:    "Testraum",
		Kameras: []speicher.Kameradaten{{Name: "PTZ Mitte", Adresse: "192.168.1.50:52381", Kanal: 1}},
		Plaetze: []speicher.Platzdaten{
			{Nummer: 1, Name: "Vorsitz", Kamera: "PTZ Mitte", Preset: 1},
			{Nummer: 2, Name: "Platz 2", Kamera: "PTZ Mitte", Preset: 2},
			{Nummer: 3, Name: "Platz 3", Kamera: "PTZ Mitte", Preset: 3},
		},
	}
}

// TestEreigniskette: hundert Ereignisse hintereinander, die Kette ist prüfbar
// und folge_nr hat keine Lücke.
func TestEreigniskette(t *testing.T) {
	for name, bauen := range ablagen(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ablage := bauen(t)

			saalID, _, err := ablage.SaalImportieren(ctx, testsaal())
			if err != nil {
				t.Fatalf("saal einlesen: %v", err)
			}

			for i := 1; i <= 100; i++ {
				e, err := ablage.EreignisAnfuegen(ctx, saalID, "mikro_an", map[string]any{
					"platz": (i % 3) + 1,
					"lauf":  i,
				})
				if err != nil {
					t.Fatalf("ereignis %d: %v", i, err)
				}
				if e.FolgeNr != int64(i) {
					t.Fatalf("ereignis %d: folge_nr %d bekommen", i, e.FolgeNr)
				}
				if i == 1 && e.VorgaengerHash != nil {
					t.Error("das erste ereignis darf keinen vorgaenger_hash haben")
				}
				if i > 1 && len(e.VorgaengerHash) == 0 {
					t.Errorf("ereignis %d hat keinen vorgaenger_hash", i)
				}
			}

			kette, err := ablage.Ereignisse(ctx, saalID)
			if err != nil {
				t.Fatalf("kette lesen: %v", err)
			}
			if len(kette) != 100 {
				t.Fatalf("100 ereignisse erwartet, %d bekommen", len(kette))
			}
			if err := kern.KettePruefen(kette); err != nil {
				t.Fatalf("kette nicht in ordnung: %v", err)
			}
			for i, e := range kette {
				if e.FolgeNr != int64(i+1) {
					t.Fatalf("lücke: an stelle %d steht folge_nr %d", i+1, e.FolgeNr)
				}
			}
		})
	}
}

// TestKetteMerktManipulation: ein geänderter Eintrag fällt beim Prüfen auf.
func TestKetteMerktManipulation(t *testing.T) {
	ctx := context.Background()
	ablage := speicher.NeuGedaechtnis()
	saalID, _, err := ablage.SaalImportieren(ctx, testsaal())
	if err != nil {
		t.Fatalf("saal einlesen: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := ablage.EreignisAnfuegen(ctx, saalID, "mikro_an", map[string]any{"platz": 1}); err != nil {
			t.Fatalf("ereignis %d: %v", i, err)
		}
	}

	kette, err := ablage.Ereignisse(ctx, saalID)
	if err != nil {
		t.Fatalf("kette lesen: %v", err)
	}
	kette[2].Nutzlast = map[string]any{"platz": 2}
	if err := kern.KettePruefen(kette); err == nil {
		t.Fatal("die änderung an ereignis 3 hätte auffallen müssen")
	}
}

// TestSaalImportIdempotent: zweimaliger Import derselben saal.json erzeugt
// keine zusätzlichen Zeilen.
func TestSaalImportIdempotent(t *testing.T) {
	for name, bauen := range ablagen(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ablage := bauen(t)
			tabellen := []string{"organisation", "saal", "kamera", "platz", "preset"}

			saalID, aufbau, err := ablage.SaalImportieren(ctx, testsaal())
			if err != nil {
				t.Fatalf("erster import: %v", err)
			}
			vorher := zaehlen(t, ctx, ablage, tabellen)

			saalIDZwei, aufbauZwei, err := ablage.SaalImportieren(ctx, testsaal())
			if err != nil {
				t.Fatalf("zweiter import: %v", err)
			}
			nachher := zaehlen(t, ctx, ablage, tabellen)

			if saalIDZwei != saalID {
				t.Errorf("saal_id hat sich geändert: %s -> %s", saalID, saalIDZwei)
			}
			for _, tabelle := range tabellen {
				if vorher[tabelle] != nachher[tabelle] {
					t.Errorf("tabelle %s: %d zeilen vor, %d nach dem zweiten import",
						tabelle, vorher[tabelle], nachher[tabelle])
				}
			}
			if len(aufbauZwei) != len(aufbau) {
				t.Fatalf("%d plätze vor, %d nach dem zweiten import", len(aufbau), len(aufbauZwei))
			}
			for i := range aufbau {
				if aufbau[i] != aufbauZwei[i] {
					t.Errorf("platz %d unterscheidet sich: %+v gegen %+v",
						aufbau[i].Nummer, aufbau[i], aufbauZwei[i])
				}
			}

			erwartet := kern.Platzaufbau{
				Nummer: 1, Name: "Vorsitz", KameraName: "PTZ Mitte",
				KameraAdresse: "192.168.1.50:52381", Kanal: 1, Preset: 1,
			}
			if aufbau[0] != erwartet {
				t.Errorf("platz 1 falsch aufgebaut: %+v", aufbau[0])
			}
		})
	}
}

// TestSaalDateiPruefung: eine unstimmige Saaldatei wird abgewiesen.
func TestSaalDateiPruefung(t *testing.T) {
	faelle := map[string]speicher.Saaldaten{
		"ohne name":         {Plaetze: []speicher.Platzdaten{{Nummer: 1, Name: "A", Kamera: "K"}}},
		"ohne platz":        {Saal: "Testraum"},
		"unbekannte kamera": {Saal: "T", Plaetze: []speicher.Platzdaten{{Nummer: 1, Name: "A", Kamera: "Fehlt"}}},
		"platznummer doppelt": {
			Saal:    "T",
			Kameras: []speicher.Kameradaten{{Name: "K", Adresse: "a:1", Kanal: 1}},
			Plaetze: []speicher.Platzdaten{
				{Nummer: 1, Name: "A", Kamera: "K"},
				{Nummer: 1, Name: "B", Kamera: "K"},
			},
		},
	}
	for name, daten := range faelle {
		t.Run(name, func(t *testing.T) {
			if err := daten.Pruefen(); err == nil {
				t.Error("die saaldatei hätte abgewiesen werden müssen")
			}
		})
	}

	gelesen, err := speicher.SaalLesen("../../saal.json")
	if err != nil {
		t.Fatalf("saal.json aus dem wurzelverzeichnis nicht lesbar: %v", err)
	}
	if len(gelesen.Plaetze) != 3 {
		t.Errorf("3 plätze in saal.json erwartet, %d gefunden", len(gelesen.Plaetze))
	}
}

func zaehlen(t *testing.T, ctx context.Context, ablage speicher.Ablage, tabellen []string) map[string]int {
	t.Helper()
	stand := make(map[string]int, len(tabellen))
	for _, tabelle := range tabellen {
		anzahl, err := ablage.Zaehlen(ctx, tabelle)
		if err != nil {
			t.Fatalf("%s zählen: %v", tabelle, err)
		}
		if anzahl == 0 {
			t.Fatalf("tabelle %s ist nach dem import leer", tabelle)
		}
		stand[tabelle] = anzahl
	}
	return stand
}

// TestSitzungImportIdempotent: zweimaliger Import derselben sitzung.json
// erzeugt keine zusätzlichen Zeilen, und die PIN steht nirgends im Klartext.
func TestSitzungImportIdempotent(t *testing.T) {
	for name, bauen := range ablagen(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ablage := bauen(t)
			tabellen := []string{"person", "sitzung", "teilnahme"}

			saalID, _, err := ablage.SaalImportieren(ctx, testsaal())
			if err != nil {
				t.Fatalf("saal einlesen: %v", err)
			}

			erste, err := ablage.SitzungImportieren(ctx, saalID, testsitzung())
			if err != nil {
				t.Fatalf("erster import: %v", err)
			}
			vorher := zaehlen(t, ctx, ablage, tabellen)

			zweite, err := ablage.SitzungImportieren(ctx, saalID, testsitzung())
			if err != nil {
				t.Fatalf("zweiter import: %v", err)
			}
			nachher := zaehlen(t, ctx, ablage, tabellen)

			if zweite.SitzungID != erste.SitzungID {
				t.Errorf("sitzung_id hat sich geändert: %s -> %s", erste.SitzungID, zweite.SitzungID)
			}
			for _, tabelle := range tabellen {
				if vorher[tabelle] != nachher[tabelle] {
					t.Errorf("tabelle %s: %d zeilen vor, %d nach dem zweiten import",
						tabelle, vorher[tabelle], nachher[tabelle])
				}
			}
			if len(erste.Teilnahmen) != 3 {
				t.Fatalf("3 teilnahmen erwartet, %d bekommen", len(erste.Teilnahmen))
			}

			// Die PIN darf nirgends im Klartext liegen, und der Hash muss
			// über beide Importe hinweg zur PIN passen.
			for i, teilnahme := range zweite.Teilnahmen {
				if bytes.Contains(teilnahme.PinHash, []byte(testsitzung().Teilnahmen[i].Pin)) {
					t.Errorf("platz %d: die pin steht im hash", teilnahme.PlatzNummer)
				}
				if err := bcrypt.CompareHashAndPassword(teilnahme.PinHash,
					[]byte(testsitzung().Teilnahmen[i].Pin)); err != nil {
					t.Errorf("platz %d: die pin passt nach dem zweiten import nicht mehr", teilnahme.PlatzNummer)
				}
			}
			if erste.Teilnahmen[0].Rolle != kern.RolleLeitung {
				t.Errorf("platz 1 sollte die leitung sein, ist %s", erste.Teilnahmen[0].Rolle)
			}
		})
	}
}

// TestTagesordnungUeberlebtDenImport: der Zustand eines aufgerufenen Punktes
// darf beim erneuten Einlesen der Sitzungsdatei nicht auf „offen"
// zurückspringen — sonst verlöre ein Serverneustart mitten in der Sitzung den
// Stand der Tagesordnung.
func TestTagesordnungUeberlebtDenImport(t *testing.T) {
	for name, bauen := range ablagen(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ablage := bauen(t)

			saalID, _, err := ablage.SaalImportieren(ctx, testsaal())
			if err != nil {
				t.Fatalf("saal einlesen: %v", err)
			}
			erste, err := ablage.SitzungImportieren(ctx, saalID, testsitzung())
			if err != nil {
				t.Fatalf("erster import: %v", err)
			}
			if len(erste.Tagesordnung) != 2 {
				t.Fatalf("2 punkte erwartet, %d bekommen", len(erste.Tagesordnung))
			}
			if erste.Tagesordnung[1].Oeffentlich {
				t.Error("punkt 2 ist als nicht öffentlich eingetragen")
			}

			if err := ablage.TopZustandSetzen(ctx, erste.Tagesordnung[1].ID, kern.TopLaufend); err != nil {
				t.Fatalf("punkt aufrufen: %v", err)
			}

			zweite, err := ablage.SitzungImportieren(ctx, saalID, testsitzung())
			if err != nil {
				t.Fatalf("zweiter import: %v", err)
			}
			if anzahl := zaehlen(t, ctx, ablage, []string{"tagesordnungspunkt"}); anzahl["tagesordnungspunkt"] != 2 {
				t.Errorf("der zweite import hat punkte verdoppelt: %d", anzahl["tagesordnungspunkt"])
			}
			if zweite.Tagesordnung[1].Zustand != kern.TopLaufend {
				t.Errorf("punkt 2 sollte weiter laufen, ist %s", zweite.Tagesordnung[1].Zustand)
			}
			if zweite.Tagesordnung[0].Zustand != kern.TopOffen {
				t.Errorf("punkt 1 sollte offen sein, ist %s", zweite.Tagesordnung[0].Zustand)
			}
		})
	}
}

func nichtOeffentlich() *bool { falsch := false; return &falsch }

func testsitzung() speicher.Sitzungsdaten {
	return speicher.Sitzungsdaten{
		Titel: "Probesitzung",
		Tagesordnung: []speicher.Topdaten{
			{Nummer: 1, Titel: "Begrüßung"},
			{Nummer: 2, Titel: "Personalangelegenheit", Oeffentlich: nichtOeffentlich()},
		},
		Teilnahmen: []speicher.Teilnahmedaten{
			{Platz: 1, Person: "Anke Bergmann", Rolle: "leitung", Pin: "1234"},
			{Platz: 2, Person: "Jonas Öztürk", Rolle: "delegierter", Pin: "2345"},
			{Platz: 3, Person: "Rita Falk", Rolle: "schriftfuehrung", Pin: "3456", Widerspruch: true},
		},
	}
}

// TestSitzordnungUeberlebtDenImport: der Saalplan braucht Geometrie. Kommt sie
// nicht durch, zeigt die Oberfläche eine Kachelreihe statt eines Plans — und
// niemand merkt, dass etwas fehlt.
func TestSitzordnungUeberlebtDenImport(t *testing.T) {
	for name, bauen := range ablagen(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ablage := bauen(t)

			saal := testsaal()
			saal.Plaetze[0].Reihe, saal.Plaetze[0].Spalte = "links", 1
			saal.Plaetze[1].Reihe, saal.Plaetze[1].Spalte = "oben", 1
			// Platz 3 bleibt ohne Geometrie — das muss erlaubt sein.

			_, aufbau, err := ablage.SaalImportieren(ctx, saal)
			if err != nil {
				t.Fatalf("saal einlesen: %v", err)
			}
			nach := map[int]kern.Platzaufbau{}
			for _, p := range aufbau {
				nach[p.Nummer] = p
			}
			if nach[1].Reihe != "links" || nach[1].Spalte != 1 {
				t.Errorf("platz 1: links/1 erwartet, %q/%d bekommen", nach[1].Reihe, nach[1].Spalte)
			}
			if nach[2].Reihe != "oben" {
				t.Errorf("platz 2: oben erwartet, %q bekommen", nach[2].Reihe)
			}
			if nach[3].Reihe != "" {
				t.Errorf("platz 3 sollte ohne reihe bleiben, hat %q", nach[3].Reihe)
			}

			// Und ein zweiter Import ändert die Geometrie mit.
			saal.Plaetze[1].Reihe = "unten"
			_, erneut, err := ablage.SaalImportieren(ctx, saal)
			if err != nil {
				t.Fatalf("zweiter import: %v", err)
			}
			for _, p := range erneut {
				if p.Nummer == 2 && p.Reihe != "unten" {
					t.Errorf("platz 2 nach dem zweiten import: unten erwartet, %q bekommen", p.Reihe)
				}
			}
		})
	}
}

// TestUnbekannteReiheWirdAbgewiesen: ein Tippfehler in der Sitzordnung darf
// nicht zu einem stillschweigend falschen Plan führen.
func TestUnbekannteReiheWirdAbgewiesen(t *testing.T) {
	saal := testsaal()
	saal.Plaetze[0].Reihe = "mitte"
	if err := saal.Pruefen(); err == nil {
		t.Error("die reihe \"mitte\" wurde angenommen")
	}

	saal = testsaal()
	saal.Plaetze[0].Spalte = 3
	if err := saal.Pruefen(); err == nil {
		t.Error("eine spalte ohne reihe wurde angenommen")
	}
}
