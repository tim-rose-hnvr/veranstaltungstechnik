package siegel_test

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/siegel"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
)

func aufbauen(t *testing.T, eintraege int) (*siegel.Siegler, *speicher.Gedaechtnis, string, *siegel.Schluessel) {
	t.Helper()
	ctx := context.Background()
	ablage := speicher.NeuGedaechtnis()

	saalID, _, err := ablage.SaalImportieren(ctx, speicher.Saaldaten{
		Saal:    "Testraum",
		Kameras: []speicher.Kameradaten{{Name: "PTZ", Adresse: "127.0.0.1:52381", Kanal: 1}},
		Plaetze: []speicher.Platzdaten{{Nummer: 1, Name: "Vorsitz", Kamera: "PTZ", Preset: 1}},
	})
	if err != nil {
		t.Fatalf("saal: %v", err)
	}
	for i := 0; i < eintraege; i++ {
		if _, err := ablage.EreignisAnfuegen(ctx, saalID, "probe", map[string]any{"nr": i}); err != nil {
			t.Fatalf("ereignis: %v", err)
		}
	}

	schluessel, err := siegel.Laden(filepath.Join(t.TempDir(), "kette.key"))
	if err != nil {
		t.Fatalf("schlüssel: %v", err)
	}
	return siegel.Neu(saalID, schluessel, ablage, nil), ablage, saalID, schluessel
}

// TestSiegelGehtAuf: das gesetzte Siegel wird von der Prüfung anerkannt.
func TestSiegelGehtAuf(t *testing.T) {
	siegler, ablage, saalID, schluessel := aufbauen(t, 5)
	ctx := context.Background()

	abschluss, err := siegler.Siegeln(ctx)
	if err != nil {
		t.Fatalf("siegeln: %v", err)
	}
	if !abschluss.Neu || abschluss.Von != 1 || abschluss.Bis != 5 {
		t.Fatalf("1 bis 5 erwartet: %+v", abschluss)
	}

	kette, err := ablage.Ereignisse(ctx, saalID)
	if err != nil {
		t.Fatalf("kette: %v", err)
	}
	bericht := siegel.Pruefen(saalID, kette, schluessel.Oeffentlich)
	if !bericht.Ok() {
		t.Fatalf("die prüfung schlägt fehl: %v", bericht.Fehler)
	}
	if bericht.Siegel != 1 || bericht.Gedeckt != 5 {
		t.Errorf("ein siegel bis 5 erwartet: %+v", bericht)
	}
	// Das Siegel selbst ist der sechste Eintrag und damit noch ungedeckt.
	if bericht.Laenge != 6 || bericht.Ungedeckt() != 1 {
		t.Errorf("kette 6 lang, 1 ungedeckt erwartet: %+v", bericht)
	}
}

// TestZweitesSiegelSchliesstNurDenRest.
func TestZweitesSiegelSchliesstNurDenRest(t *testing.T) {
	siegler, ablage, saalID, _ := aufbauen(t, 3)
	ctx := context.Background()

	if _, err := siegler.Siegeln(ctx); err != nil {
		t.Fatalf("erstes siegel: %v", err)
	}
	// Ohne neue Einträge wird kein zweites gesetzt.
	leer, err := siegler.Siegeln(ctx)
	if err != nil {
		t.Fatalf("zweites siegel: %v", err)
	}
	if leer.Neu {
		t.Error("ein siegel ohne neue einträge verlängert nur die kette")
	}

	if _, err := ablage.EreignisAnfuegen(ctx, saalID, "probe", map[string]any{"nr": 99}); err != nil {
		t.Fatalf("ereignis: %v", err)
	}
	zweites, err := siegler.Siegeln(ctx)
	if err != nil {
		t.Fatalf("drittes siegel: %v", err)
	}
	// Es setzt bei 4 an, nicht bei 5: das erste Siegel selbst ist Eintrag 4
	// und muss mitgedeckt werden, sonst bliebe zwischen den Siegeln eine
	// ungedeckte Stelle, an der sich etwas einschieben ließe.
	if !zweites.Neu || zweites.Von != 4 || zweites.Bis != 5 {
		t.Errorf("das zweite siegel deckt 4 bis 5: %+v", zweites)
	}

	// Und danach ist die ganze Kette bis auf das letzte Siegel gedeckt.
	kette, err := ablage.Ereignisse(ctx, saalID)
	if err != nil {
		t.Fatalf("kette: %v", err)
	}
	bericht := siegel.Pruefen(saalID, kette, nil)
	if !bericht.Ok() {
		t.Fatalf("prüfung: %v", bericht.Fehler)
	}
	if bericht.Ungedeckt() != 1 {
		t.Errorf("nur das letzte siegel darf ungedeckt sein: %+v", bericht)
	}
}

// TestNachgebauteKetteFaelltAuf: genau dafür gibt es das Siegel. Wer die Kette
// neu rechnet, kann sie in sich stimmig machen — aber nicht unterschreiben.
func TestNachgebauteKetteFaelltAuf(t *testing.T) {
	siegler, ablage, saalID, schluessel := aufbauen(t, 4)
	ctx := context.Background()
	if _, err := siegler.Siegeln(ctx); err != nil {
		t.Fatalf("siegeln: %v", err)
	}
	echte, err := ablage.Ereignisse(ctx, saalID)
	if err != nil {
		t.Fatalf("kette: %v", err)
	}

	// Jemand baut die Kette neu: ein Eintrag bekommt eine andere Nutzlast,
	// alle Hashes werden neu gerechnet. Die Kette selbst ist danach in Ordnung.
	nachgebaut := make([]kern.Ereignis, 0, len(echte))
	var vorher *kern.Ereignis
	for i, e := range echte {
		nutzlast := e.Nutzlast
		if i == 1 {
			nutzlast = map[string]any{"nr": 4711}
		}
		neu, err := kern.NaechstesEreignis(vorher, e.Zeit, e.Art, nutzlast)
		if err != nil {
			t.Fatalf("nachbauen: %v", err)
		}
		nachgebaut = append(nachgebaut, neu)
		vorher = &nachgebaut[len(nachgebaut)-1]
	}
	if err := kern.KettePruefen(nachgebaut); err != nil {
		t.Fatalf("die nachgebaute kette sollte in sich stimmen: %v", err)
	}

	bericht := siegel.Pruefen(saalID, nachgebaut, schluessel.Oeffentlich)
	if bericht.Ok() {
		t.Fatal("die nachgebaute kette hat die siegelprüfung bestanden")
	}
	if !strings.Contains(strings.Join(bericht.Fehler, " "), "ausgetauscht") {
		t.Errorf("der fehler benennt den austausch nicht: %v", bericht.Fehler)
	}
}

// TestFremderSchluesselWirdAbgewiesen: ein selbst gesetztes Siegel nützt
// nichts, wenn der Schlüssel nicht der erwartete ist.
func TestFremderSchluesselWirdAbgewiesen(t *testing.T) {
	siegler, ablage, saalID, _ := aufbauen(t, 3)
	ctx := context.Background()
	if _, err := siegler.Siegeln(ctx); err != nil {
		t.Fatalf("siegeln: %v", err)
	}
	kette, err := ablage.Ereignisse(ctx, saalID)
	if err != nil {
		t.Fatalf("kette: %v", err)
	}

	fremd, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("schlüssel: %v", err)
	}
	bericht := siegel.Pruefen(saalID, kette, fremd)
	if bericht.Ok() {
		t.Fatal("ein siegel mit fremdem schlüssel wurde anerkannt")
	}
	if !strings.Contains(strings.Join(bericht.Fehler, " "), "fremden schlüssel") {
		t.Errorf("der fehler benennt den fremden schlüssel nicht: %v", bericht.Fehler)
	}
}

// TestSaalGehtInDieUnterschrift: ein Siegel aus einem anderen Saal passt nicht.
func TestSaalGehtInDieUnterschrift(t *testing.T) {
	siegler, ablage, saalID, schluessel := aufbauen(t, 3)
	ctx := context.Background()
	if _, err := siegler.Siegeln(ctx); err != nil {
		t.Fatalf("siegeln: %v", err)
	}
	kette, err := ablage.Ereignisse(ctx, saalID)
	if err != nil {
		t.Fatalf("kette: %v", err)
	}
	if bericht := siegel.Pruefen("ein-anderer-saal", kette, schluessel.Oeffentlich); bericht.Ok() {
		t.Fatal("das siegel gilt auch für einen anderen saal")
	}
}

// TestSchluesselDateiRechte: ein Schlüssel, den jeder lesen kann, beweist nichts.
func TestSchluesselDateiRechte(t *testing.T) {
	ordner := t.TempDir()
	pfad := filepath.Join(ordner, "kette.key")

	erst, err := siegel.Laden(pfad)
	if err != nil {
		t.Fatalf("anlegen: %v", err)
	}
	lage, err := os.Stat(pfad)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if lage.Mode().Perm() != 0o600 {
		t.Errorf("0600 erwartet, %o bekommen", lage.Mode().Perm())
	}

	// Derselbe Schlüssel beim zweiten Laden.
	zweit, err := siegel.Laden(pfad)
	if err != nil {
		t.Fatalf("erneut laden: %v", err)
	}
	if !erst.Oeffentlich.Equal(zweit.Oeffentlich) {
		t.Error("das zweite laden erzeugte einen anderen schlüssel")
	}

	// Der öffentliche Teil liegt daneben und passt.
	roh, err := os.ReadFile(pfad + ".pub")
	if err != nil {
		t.Fatalf("öffentlicher schlüssel: %v", err)
	}
	if hex.EncodeToString(erst.Oeffentlich) != strings.TrimSpace(string(roh)) {
		t.Error("der abgelegte öffentliche schlüssel passt nicht zum privaten")
	}

	if err := os.Chmod(pfad, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := siegel.Laden(pfad); err == nil {
		t.Error("ein weltweit lesbarer siegelschlüssel wurde angenommen")
	}
}

// TestFingerabdruckIstKurzUndStabil: er wird von Hand verglichen.
func TestFingerabdruckIstKurzUndStabil(t *testing.T) {
	oeffentlich, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("schlüssel: %v", err)
	}
	abdruck := siegel.Fingerabdruck(oeffentlich)
	if len(abdruck) != 16 {
		t.Errorf("16 zeichen erwartet, %d bekommen: %q", len(abdruck), abdruck)
	}
	if abdruck != siegel.Fingerabdruck(oeffentlich) {
		t.Error("der fingerabdruck ist nicht stabil")
	}
}
