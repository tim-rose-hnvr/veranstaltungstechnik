package kern_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
)

// mappe legt eine Sitzungsmappe an: je eine Unterlage pro Stufe, auf Punkt 1.
func mappe(t *testing.T) (*pruefstand, string) {
	t.Helper()
	ordner := t.TempDir()

	for datei, inhalt := range map[string]string{
		"oeffentlich.pdf": "für alle",
		"intern.pdf":      "für Teilnehmende",
		"vertraulich.pdf": "für Stimmberechtigte",
		"geheim.pdf":      "nur für die Leitung",
	} {
		if err := os.WriteFile(filepath.Join(ordner, datei), []byte(inhalt), 0o600); err != nil {
			t.Fatalf("unterlage schreiben: %v", err)
		}
	}

	tagesordnung := []speicher.Topdaten{{
		Nummer: 1, Titel: "Bericht",
		Unterlagen: []speicher.Unterlagedaten{
			{Titel: "Für alle", Datei: "oeffentlich.pdf", Stufe: "oeffentlich"},
			{Titel: "Für Teilnehmende", Datei: "intern.pdf", Stufe: "intern"},
			{Titel: "Für Stimmberechtigte", Datei: "vertraulich.pdf", Stufe: "vertraulich"},
			{Titel: "Nur für die Leitung", Datei: "geheim.pdf", Stufe: "geheim"},
		},
	}}

	besetzung := append(standardbesetzung(),
		speicher.Teilnahmedaten{Platz: 6, Person: "Ali Kaya", Rolle: "gast", Pin: "6666"})
	p := aufbauenMitMappe(t, ordner, besetzung, tagesordnung)
	return p, ordner
}

// TestStufenLeiter: jede Stufe unterscheidet sich tatsächlich von der nächsten.
func TestStufenLeiter(t *testing.T) {
	erwartet := map[kern.Rolle][]kern.Stufe{
		kern.RolleLeitung:         {kern.StufeOeffentlich, kern.StufeIntern, kern.StufeVertraulich, kern.StufeGeheim},
		kern.RolleDelegierter:     {kern.StufeOeffentlich, kern.StufeIntern, kern.StufeVertraulich},
		kern.RolleSchriftfuehrung: {kern.StufeOeffentlich, kern.StufeIntern},
		kern.RolleGast:            {kern.StufeOeffentlich},
	}
	alle := []kern.Stufe{kern.StufeOeffentlich, kern.StufeIntern, kern.StufeVertraulich, kern.StufeGeheim}

	for rolle, erlaubt := range erwartet {
		darf := map[kern.Stufe]bool{}
		for _, s := range erlaubt {
			darf[s] = true
		}
		for _, s := range alle {
			if got := s.DarfSehen(rolle); got != darf[s] {
				t.Errorf("rolle %s, stufe %s: %v erwartet, %v bekommen", rolle, s, darf[s], got)
			}
		}
	}

	// Eine unbekannte Stufe ist die strengste. Ein Tippfehler in der
	// Sitzungsdatei darf nichts öffnen.
	if kern.Stufe("vertrulich").DarfSehen(kern.RolleLeitung) {
		t.Error("eine unbekannte stufe muss verschlossen bleiben")
	}
}

// TestMappeZeigtNurWasErlaubtIst: was die Rolle nicht sehen darf, steht nicht
// in der Liste — es wird nicht ausgegraut, sondern nie gesendet.
func TestMappeZeigtNurWasErlaubtIst(t *testing.T) {
	p, _ := mappe(t)
	anmeldenAlle(t, p, 1, 2, 3)
	if err := p.kern.Anmelden(context.Background(), 6, "6666"); err != nil {
		t.Fatalf("gast anmelden: %v", err)
	}

	for platz, anzahl := range map[int]int{1: 4, 2: 3, 3: 2, 6: 1} {
		sichtbar := p.kern.UnterlagenFuer(platz)
		if len(sichtbar) != anzahl {
			titel := make([]string, 0, len(sichtbar))
			for _, u := range sichtbar {
				titel = append(titel, u.Titel)
			}
			t.Errorf("platz %d: %d unterlagen erwartet, %d bekommen: %v", platz, anzahl, len(sichtbar), titel)
		}
	}

	// Und im Ich-Zustand steht dasselbe — die Oberfläche bekommt nichts anderes.
	if ich := p.kern.Ich(3); ich == nil || len(ich.Unterlagen) != 2 {
		t.Errorf("die schriftführung sieht im ich-zustand nicht zwei unterlagen: %+v", ich)
	}
}

// TestVerweigerterAbrufStehtImProtokoll: wer eine Unterlage anfragt, die er
// nicht sehen darf, ist ein Vorgang.
func TestVerweigerterAbrufStehtImProtokoll(t *testing.T) {
	p, _ := mappe(t)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 3)

	geheim := unterlageMit(t, p, 1, "Nur für die Leitung")
	if code := codeVon(t, abrufFehler(p.kern.UnterlageAbrufen(ctx, 3, geheim))); code != kern.CodeNichtBerechtigt {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeNichtBerechtigt, code)
	}
	if !hatEreignis(t, p, "unterlage_verweigert") {
		t.Error("der abgewiesene versuch steht nicht im protokoll")
	}
	if hatEreignis(t, p, "unterlage_geoeffnet") {
		t.Error("es steht ein öffnen im protokoll, obwohl abgewiesen wurde")
	}
}

// TestAbrufStehtMitPersonUndZeitImProtokoll: das Zugriffsprotokoll ist die
// Ereigniskette selbst.
func TestAbrufStehtMitPersonUndZeitImProtokoll(t *testing.T) {
	p, _ := mappe(t)
	ctx := context.Background()
	anmeldenAlle(t, p, 1)
	eroeffnen(t, p)

	id := unterlageMit(t, p, 1, "Für Stimmberechtigte")
	freigabe, err := p.kern.UnterlageAbrufen(ctx, 1, id)
	if err != nil {
		t.Fatalf("abrufen: %v", err)
	}

	kette, err := p.ablage.Ereignisse(ctx, p.saalID)
	if err != nil {
		t.Fatalf("kette lesen: %v", err)
	}
	gefunden := false
	for _, e := range kette {
		if e.Art != "unterlage_geoeffnet" {
			continue
		}
		gefunden = true
		if text(e.Nutzlast["person"]) != "Anke Bergmann" || zahlAus(e.Nutzlast["platz"]) != 1 {
			t.Errorf("person und platz fehlen im eintrag: %+v", e.Nutzlast)
		}
		if text(e.Nutzlast["stufe"]) != string(kern.StufeVertraulich) {
			t.Errorf("die stufe fehlt im eintrag: %+v", e.Nutzlast)
		}
		if _, dabei := e.Nutzlast["ms"]; !dabei {
			t.Errorf("der eintrag liegt auf keiner zeitachse: %+v", e.Nutzlast)
		}
	}
	if !gefunden {
		t.Fatal("kein unterlage_geoeffnet in der kette")
	}

	// Das Wasserzeichen benennt den Empfänger.
	for _, teil := range []string{"Anke Bergmann", "Platz 1"} {
		if !strings.Contains(freigabe.Wasserzeichen, teil) {
			t.Errorf("das wasserzeichen %q nennt %q nicht", freigabe.Wasserzeichen, teil)
		}
	}
}

// TestMarkeGiltEinmal: ein weitergegebener Verweis nützt nichts.
func TestMarkeGiltEinmal(t *testing.T) {
	p, _ := mappe(t)
	ctx := context.Background()
	anmeldenAlle(t, p, 1)

	freigabe, err := p.kern.UnterlageAbrufen(ctx, 1, unterlageMit(t, p, 1, "Für alle"))
	if err != nil {
		t.Fatalf("abrufen: %v", err)
	}

	inhalt, _, _, err := p.kern.UnterlageOeffnen(freigabe.Marke)
	if err != nil {
		t.Fatalf("erster abruf: %v", err)
	}
	roh, _ := io.ReadAll(inhalt)
	inhalt.Close()
	if string(roh) != "für alle" {
		t.Errorf("inhalt %q bekommen", roh)
	}

	if _, _, _, err := p.kern.UnterlageOeffnen(freigabe.Marke); err == nil {
		t.Error("die marke ließ sich ein zweites mal einlösen")
	} else if code := codeVon(t, err); code != kern.CodeFreigabeAbgelaufen {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeFreigabeAbgelaufen, code)
	}

	// Und eine erfundene Marke öffnet nichts.
	if _, _, _, err := p.kern.UnterlageOeffnen("00000000000000000000000000000000"); err == nil {
		t.Error("eine erfundene marke öffnete eine unterlage")
	}
}

// TestAusgetauschteDateiWirdNichtAusgeliefert: eine Unterlage, die nicht mehr
// die eingelesene ist, gehört nicht in eine Sitzung.
func TestAusgetauschteDateiWirdNichtAusgeliefert(t *testing.T) {
	p, ordner := mappe(t)
	ctx := context.Background()
	anmeldenAlle(t, p, 1)

	freigabe, err := p.kern.UnterlageAbrufen(ctx, 1, unterlageMit(t, p, 1, "Für alle"))
	if err != nil {
		t.Fatalf("abrufen: %v", err)
	}
	// Jemand tauscht die Datei unter dem laufenden System aus.
	if err := os.WriteFile(filepath.Join(ordner, "oeffentlich.pdf"), []byte("etwas anderes"), 0o600); err != nil {
		t.Fatalf("datei austauschen: %v", err)
	}

	if _, _, _, err := p.kern.UnterlageOeffnen(freigabe.Marke); err == nil {
		t.Fatal("die ausgetauschte datei wurde ausgeliefert")
	} else if code := codeVon(t, err); code != kern.CodeUnterlageVeraendert {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeUnterlageVeraendert, code)
	}
}

// TestFreigabeLaeuftAb: die Marke ist kurzlebig.
func TestFreigabeLaeuftAb(t *testing.T) {
	if kern.FreigabeDauer < time.Second {
		t.Fatalf("die freigabedauer ist mit %s unbrauchbar kurz", kern.FreigabeDauer)
	}
	if kern.FreigabeDauer > 5*time.Minute {
		t.Errorf("die freigabedauer ist mit %s zu lang für eine einmalige marke", kern.FreigabeDauer)
	}
}

// TestFehlendeUnterlageBrichtDenStartAb: wer eine Mappe angibt, will sie auch
// ausliefern. Eine fehlende Datei ist ein Startfehler, keine leere Zeile.
func TestFehlendeUnterlageBrichtDenStartAb(t *testing.T) {
	ordner := t.TempDir()
	ablage := speicher.NeuGedaechtnis()
	ctx := context.Background()

	saal := speicher.Saaldaten{
		Saal:    "Testraum",
		Kameras: []speicher.Kameradaten{{Name: "PTZ Mitte", Adresse: "127.0.0.1:52381", Kanal: 1}},
		Plaetze: []speicher.Platzdaten{{Nummer: 1, Name: "Vorsitz", Kamera: "PTZ Mitte", Preset: 1}},
	}
	saalID, _, err := ablage.SaalImportieren(ctx, saal)
	if err != nil {
		t.Fatalf("saal einlesen: %v", err)
	}

	daten := speicher.Sitzungsdaten{
		Titel: "Probesitzung",
		Tagesordnung: []speicher.Topdaten{{Nummer: 1, Titel: "Bericht",
			Unterlagen: []speicher.Unterlagedaten{{Datei: "gibtesnicht.pdf"}}}},
		Teilnahmen: []speicher.Teilnahmedaten{
			{Platz: 1, Person: "Anke Bergmann", Rolle: "leitung", Pin: "1111"},
		},
	}.MitVerzeichnis(ordner)

	if _, err := ablage.SitzungImportieren(ctx, saalID, daten); err == nil {
		t.Fatal("eine fehlende unterlage wurde stillschweigend übergangen")
	}
}

// TestPfadDarfNichtHinausfuehren: ".." in einem Pfad ist keine Unterlage,
// sondern ein Versuch.
func TestPfadDarfNichtHinausfuehren(t *testing.T) {
	for _, pfad := range []string{"../geheim.pdf", "unter/../../weg.pdf", "/etc/passwd"} {
		daten := speicher.Sitzungsdaten{
			Titel: "Probesitzung",
			Tagesordnung: []speicher.Topdaten{{Nummer: 1, Titel: "Bericht",
				Unterlagen: []speicher.Unterlagedaten{{Datei: pfad}}}},
			Teilnahmen: []speicher.Teilnahmedaten{
				{Platz: 1, Person: "Anke Bergmann", Rolle: "leitung", Pin: "1111"},
			},
		}
		if err := daten.Pruefen(); err == nil {
			t.Errorf("der pfad %q wurde angenommen", pfad)
		}
	}
}

// unterlageMit findet die Kennung einer Unterlage über ihren Titel.
func unterlageMit(t *testing.T, p *pruefstand, platz int, titel string) string {
	t.Helper()
	for _, u := range p.kern.UnterlagenFuer(platz) {
		if u.Titel == titel {
			return u.ID
		}
	}
	t.Fatalf("platz %d sieht keine unterlage %q", platz, titel)
	return ""
}

func abrufFehler(_ *kern.Freigabe, err error) error { return err }

func text(wert any) string {
	if s, passt := wert.(string); passt {
		return s
	}
	return ""
}
