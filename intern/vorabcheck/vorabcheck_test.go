package vorabcheck_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/siegel"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/vorabcheck"
)

// letzteAblage und letzterAufbau merken sich, was aufbauen zuletzt gebaut hat
// — die Siegelprüfung braucht beides und der vorhandene Aufbau gibt es nicht
// heraus.
var (
	letzteAblage  *speicher.Gedaechtnis
	letzterAufbau kern.Aufbau
)

type stilleKamera struct{ antwort error }

func (s *stilleKamera) PresetAbrufen(ctx context.Context, adresse string, kanal, preset uint8) error {
	return s.antwort
}

// aufbauen stellt Saal, Sitzung und Kern über die Ablage im Arbeitsspeicher auf.
func aufbauen(t *testing.T, teilnahmen []speicher.Teilnahmedaten, kameraFehler error) (*vorabcheck.Pruefer, *kern.Kern) {
	t.Helper()
	ctx := context.Background()
	ablage := speicher.NeuGedaechtnis()

	saal := speicher.Saaldaten{
		Saal:    "Testraum",
		Kameras: []speicher.Kameradaten{{Name: "PTZ Mitte", Adresse: "192.168.1.50:52381", Kanal: 1}},
		Plaetze: []speicher.Platzdaten{
			{Nummer: 1, Name: "Vorsitz", Kamera: "PTZ Mitte", Preset: 1},
			{Nummer: 2, Name: "Platz 2", Kamera: "PTZ Mitte", Preset: 2},
		},
	}
	saalID, plaetze, err := ablage.SaalImportieren(ctx, saal)
	if err != nil {
		t.Fatalf("saal einlesen: %v", err)
	}
	stand, err := ablage.SitzungImportieren(ctx, saalID,
		speicher.Sitzungsdaten{Titel: "Probesitzung", Teilnahmen: teilnahmen})
	if err != nil {
		t.Fatalf("sitzung einlesen: %v", err)
	}

	aufbau := kern.Aufbau{
		SaalID:         saalID,
		SitzungID:      stand.SitzungID,
		Titel:          stand.Titel,
		SitzungZustand: stand.Zustand,
		Plaetze:        plaetze,
		Teilnahmen:     stand.Teilnahmen,
		MaxOffen:       2,
		Zeitlimit:      50 * time.Millisecond,
	}
	steuerung := &stilleKamera{antwort: kameraFehler}
	k, err := kern.Neu(aufbau, steuerung, ablage, nil)
	if err != nil {
		t.Fatalf("kern nicht aufgebaut: %v", err)
	}
	letzterAufbau = aufbau
	letzteAblage = ablage
	return vorabcheck.Neu(aufbau, k, steuerung, ablage, 50*time.Millisecond), k
}

func vollbesetzt() []speicher.Teilnahmedaten {
	return []speicher.Teilnahmedaten{
		{Platz: 1, Person: "Anke Bergmann", Rolle: "leitung", Pin: "1111"},
		{Platz: 2, Person: "Jonas Öztürk", Rolle: "delegierter", Pin: "2222"},
	}
}

// finde sucht einen Punkt anhand von Bereich und Titel.
func finde(t *testing.T, b vorabcheck.Bericht, bereich, titel string) vorabcheck.Punkt {
	t.Helper()
	for _, p := range b.Punkte {
		if p.Bereich == bereich && p.Titel == titel {
			return p
		}
	}
	t.Fatalf("punkt %q/%q fehlt im bericht", bereich, titel)
	return vorabcheck.Punkt{}
}

// TestVorabcheckMeldetBereit: ist alles besetzt und die Kamera antwortet,
// meldet der Bericht bereit.
func TestVorabcheckMeldetBereit(t *testing.T) {
	pruefer, _ := aufbauen(t, vollbesetzt(), nil)

	bericht, err := pruefer.Laufen(context.Background())
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	if !bericht.Bereit || bericht.Fehler != 0 {
		t.Fatalf("bereit erwartet, %d fehler bekommen: %+v", bericht.Fehler, bericht.Punkte)
	}
	if p := finde(t, bericht, "Platz 1", "Kamera"); p.Ergebnis != vorabcheck.Ok {
		t.Errorf("kamera sollte in ordnung sein, ist %s: %s", p.Ergebnis, p.Text)
	}
	if p := finde(t, bericht, "Protokoll", "Ereigniskette"); p.Ergebnis != vorabcheck.Ok {
		t.Errorf("kette sollte in ordnung sein, ist %s: %s", p.Ergebnis, p.Text)
	}
}

// TestVorabcheckMeldetStummeKamera: antwortet die Kamera nicht, ist der Saal
// nicht bereit — und der Text sagt, was zu tun ist.
func TestVorabcheckMeldetStummeKamera(t *testing.T) {
	pruefer, _ := aufbauen(t, vollbesetzt(), errors.New("keine antwort"))

	bericht, err := pruefer.Laufen(context.Background())
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	if bericht.Bereit {
		t.Error("mit stummer kamera darf nicht bereit gemeldet werden")
	}
	p := finde(t, bericht, "Platz 2", "Kamera")
	if p.Ergebnis != vorabcheck.Fehler {
		t.Fatalf("fehler erwartet, %s bekommen", p.Ergebnis)
	}
	// Klartext statt Messwert: der Text nennt die Adresse und was zu prüfen ist.
	if !strings.Contains(p.Text, "192.168.1.50:52381") || !strings.Contains(p.Text, "Kabel") {
		t.Errorf("der text hilft dem techniker nicht weiter: %q", p.Text)
	}
}

// TestVorabcheckMeldetLeerenPlatz: ein Platz ohne Person ist ein Hinweis,
// kein Fehler — die Sitzung kann trotzdem stattfinden.
func TestVorabcheckMeldetLeerenPlatz(t *testing.T) {
	nurEiner := []speicher.Teilnahmedaten{
		{Platz: 1, Person: "Anke Bergmann", Rolle: "leitung", Pin: "1111"},
	}
	pruefer, _ := aufbauen(t, nurEiner, nil)

	bericht, err := pruefer.Laufen(context.Background())
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	p := finde(t, bericht, "Platz 2", "Besetzung")
	if p.Ergebnis != vorabcheck.Hinweis {
		t.Errorf("hinweis erwartet, %s bekommen", p.Ergebnis)
	}
	if !bericht.Bereit {
		t.Error("ein unbesetzter platz darf die sitzung nicht verhindern")
	}
	// Nur eine Person darf leiten — darauf weist der Bericht hin.
	if p := finde(t, bericht, "Sitzung", "Sitzungsleitung"); p.Ergebnis != vorabcheck.Hinweis {
		t.Errorf("hinweis zur leitung erwartet, %s bekommen", p.Ergebnis)
	}
}

// TestVorabcheckGesperrtWaehrendDerSitzung: der Check fährt Kameras und ist
// deshalb während einer laufenden Sitzung gesperrt.
func TestVorabcheckGesperrtWaehrendDerSitzung(t *testing.T) {
	pruefer, k := aufbauen(t, vollbesetzt(), nil)
	if err := k.SitzungEroeffnen(context.Background(), 1); err != nil {
		t.Fatalf("sitzung nicht eröffnet: %v", err)
	}

	if _, err := pruefer.Laufen(context.Background()); !errors.Is(err, vorabcheck.ErrSitzungLaeuft) {
		t.Fatalf("ablehnung erwartet, %v bekommen", err)
	}
}

// TestSiegelImVorabcheck: der Vorabcheck sagt, ob die Kette abgeschlossen ist
// — und weist einen Nachbau ab.
func TestSiegelImVorabcheck(t *testing.T) {
	p, k := aufbauen(t, vollbesetzt(), nil)
	ablage, saalID := letzteAblage, letzterAufbau.SaalID
	ctx := context.Background()

	// Eine leere Kette lässt sich nicht abschließen — erst muss etwas
	// geschehen sein.
	if err := k.Anmelden(ctx, 1, "1111"); err != nil {
		t.Fatalf("anmelden: %v", err)
	}

	schluessel, err := siegel.Laden(filepath.Join(t.TempDir(), "kette.key"))
	if err != nil {
		t.Fatalf("schlüssel: %v", err)
	}

	// Ohne Schlüssel: ein Hinweis, kein Fehler.
	if punkt := punktMit(t, p, "Siegel"); punkt.Ergebnis != vorabcheck.Hinweis {
		t.Errorf("ohne schlüssel ein hinweis erwartet, %s bekommen: %s", punkt.Ergebnis, punkt.Text)
	}

	p.SetzeSiegelschluessel(schluessel.Oeffentlich)
	// Mit Schlüssel, aber ohne Abschluss: ebenfalls ein Hinweis.
	if punkt := punktMit(t, p, "Siegel"); punkt.Ergebnis != vorabcheck.Hinweis {
		t.Errorf("ohne abschluss ein hinweis erwartet, %s bekommen: %s", punkt.Ergebnis, punkt.Text)
	}

	if _, err := siegel.Neu(saalID, schluessel, ablage, nil).Siegeln(ctx); err != nil {
		t.Fatalf("siegeln: %v", err)
	}
	punkt := punktMit(t, p, "Siegel")
	if punkt.Ergebnis != vorabcheck.Ok {
		t.Errorf("nach dem abschluss ok erwartet, %s bekommen: %s", punkt.Ergebnis, punkt.Text)
	}
	if !strings.Contains(punkt.Text, siegel.Fingerabdruck(schluessel.Oeffentlich)) {
		t.Errorf("der fingerabdruck fehlt im bericht: %s", punkt.Text)
	}
}

func punktMit(t *testing.T, p *vorabcheck.Pruefer, titel string) vorabcheck.Punkt {
	t.Helper()
	bericht, err := p.Laufen(context.Background())
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	for _, punkt := range bericht.Punkte {
		if punkt.Titel == titel {
			return punkt
		}
	}
	t.Fatalf("keine prüfung %q im bericht", titel)
	return vorabcheck.Punkt{}
}

// TestEinmessungImVorabcheck: der Bericht sagt, ob die Höchstzahl offener
// Mikrofone gemessen oder nur gesetzt ist — und schlägt Alarm, wenn die
// Konfiguration mehr verlangt, als die Reserve trägt.
func TestEinmessungImVorabcheck(t *testing.T) {
	// Ohne Einmessung: ein Hinweis, der zum Ring-out schickt.
	p, _ := aufbauen(t, vollbesetzt(), nil)
	bericht, err := p.Laufen(context.Background())
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	punkt := finde(t, bericht, "Sitzung", "Einmessung")
	if punkt.Ergebnis != vorabcheck.Hinweis || !strings.Contains(punkt.Text, "Ring-out") {
		t.Errorf("ohne einmessung ein hinweis mit ring-out erwartet: %s %q", punkt.Ergebnis, punkt.Text)
	}

	// Mit 9 dB Reserve und MaxOffen 2: alles im Rahmen, der Text nennt die Zahlen.
	aufbauen(t, vollbesetzt(), nil)
	mitReserve := letzterAufbau
	mitReserve.EinmessungReserveDB = 9
	pruefer := vorabcheck.Neu(mitReserve, neuerKern(t, mitReserve), &stilleKamera{}, letzteAblage, 50*time.Millisecond)
	bericht, err = pruefer.Laufen(context.Background())
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	punkt = finde(t, bericht, "Sitzung", "Einmessung")
	if punkt.Ergebnis != vorabcheck.Ok || !strings.Contains(punkt.Text, "9.0 dB") {
		t.Errorf("mit messung ok samt zahlen erwartet: %s %q", punkt.Ergebnis, punkt.Text)
	}

	// Konfiguration über der Messung: Fehler, keine Beschönigung.
	mitReserve.EinmessungReserveDB = 3 // trägt 2 — verlangt sind 4
	mitReserve.MaxOffen = 4
	pruefer = vorabcheck.Neu(mitReserve, neuerKern(t, mitReserve), &stilleKamera{}, letzteAblage, 50*time.Millisecond)
	bericht, err = pruefer.Laufen(context.Background())
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	punkt = finde(t, bericht, "Sitzung", "Einmessung")
	if punkt.Ergebnis != vorabcheck.Fehler || !strings.Contains(punkt.Text, "Rückkopplung") {
		t.Errorf("zu viel verlangt: fehler mit rückkopplungs-warnung erwartet: %s %q", punkt.Ergebnis, punkt.Text)
	}
}

// neuerKern baut für Abwandlungen des Aufbaus einen frischen Kern.
func neuerKern(t *testing.T, aufbau kern.Aufbau) *kern.Kern {
	t.Helper()
	k, err := kern.Neu(aufbau, &stilleKamera{}, letzteAblage, nil)
	if err != nil {
		t.Fatalf("kern: %v", err)
	}
	return k
}
