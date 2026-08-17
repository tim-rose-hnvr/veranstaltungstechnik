package kern_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
)

// stilleKamera nimmt jeden Befehl an, ohne UDP zu sprechen.
type stilleKamera struct {
	mu      sync.Mutex
	abrufe  []abruf
	antwort error
}

type abruf struct {
	Adresse string
	Kanal   uint8
	Preset  uint8
}

func (s *stilleKamera) PresetAbrufen(ctx context.Context, adresse string, kanal, preset uint8) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.abrufe = append(s.abrufe, abruf{adresse, kanal, preset})
	return s.antwort
}

func (s *stilleKamera) Abrufe() []abruf {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]abruf(nil), s.abrufe...)
}

// pruefstand hält alles zusammen, was ein Test braucht.
type pruefstand struct {
	kern   *kern.Kern
	kamera *stilleKamera
	ablage speicher.Ablage
	saalID string
}

// aufbauen baut Saal, Sitzung und Kern über die Ablage im Arbeitsspeicher —
// dieselben Wege wie im Betrieb, nur ohne Datenbank und ohne UDP.
func aufbauen(t *testing.T, plaetze, maxOffen int, teilnahmen []speicher.Teilnahmedaten) *pruefstand {
	t.Helper()
	ctx := context.Background()
	ablage := speicher.NeuGedaechtnis()

	saal := speicher.Saaldaten{
		Saal:    "Testraum",
		Kameras: []speicher.Kameradaten{{Name: "PTZ Mitte", Adresse: "192.168.1.50:52381", Kanal: 1}},
	}
	for i := 1; i <= plaetze; i++ {
		saal.Plaetze = append(saal.Plaetze, speicher.Platzdaten{
			Nummer: i, Name: fmt.Sprintf("Platz %d", i), Kamera: "PTZ Mitte", Preset: uint8(i),
		})
	}
	saalID, platzaufbau, err := ablage.SaalImportieren(ctx, saal)
	if err != nil {
		t.Fatalf("saal einlesen: %v", err)
	}

	stand, err := ablage.SitzungImportieren(ctx, saalID,
		speicher.Sitzungsdaten{Titel: "Probesitzung", Teilnahmen: teilnahmen})
	if err != nil {
		t.Fatalf("sitzung einlesen: %v", err)
	}

	kamera := &stilleKamera{}
	k, err := kern.Neu(kern.Aufbau{
		SaalID:         saalID,
		SitzungID:      stand.SitzungID,
		Titel:          stand.Titel,
		SitzungZustand: stand.Zustand,
		Plaetze:        platzaufbau,
		Teilnahmen:     stand.Teilnahmen,
		MaxOffen:       maxOffen,
		Zeitlimit:      100 * time.Millisecond,
	}, kamera, ablage, nil)
	if err != nil {
		t.Fatalf("kern nicht aufgebaut: %v", err)
	}
	return &pruefstand{kern: k, kamera: kamera, ablage: ablage, saalID: saalID}
}

// standardbesetzung: zwei zur Leitung Berechtigte, ein Delegierter,
// eine Schriftführung.
func standardbesetzung() []speicher.Teilnahmedaten {
	return []speicher.Teilnahmedaten{
		{Platz: 1, Person: "Anke Bergmann", Rolle: "leitung", Pin: "1111"},
		{Platz: 2, Person: "Jonas Öztürk", Rolle: "delegierter", Pin: "2222"},
		{Platz: 3, Person: "Rita Falk", Rolle: "schriftfuehrung", Pin: "3333"},
		{Platz: 4, Person: "Mark Voss", Rolle: "leitung", Pin: "4444"},
		{Platz: 5, Person: "Ida Peters", Rolle: "delegierter", Pin: "5555"},
	}
}

// eroeffnen bringt die Sitzung ins Laufen. Platz 1 führt.
func eroeffnen(t *testing.T, p *pruefstand) {
	t.Helper()
	if err := p.kern.SitzungEroeffnen(context.Background(), 1); err != nil {
		t.Fatalf("sitzung nicht eröffnet: %v", err)
	}
}

func codeVon(t *testing.T, err error) string {
	t.Helper()
	var f *kern.Fehler
	if errors.As(err, &f) {
		return f.Code
	}
	if err == nil {
		return ""
	}
	t.Fatalf("kein fachlicher fehler: %v", err)
	return ""
}

// --- Meilenstein 1: die Regeln von damals gelten weiter ---

// TestGrenzeOffenerMikrofone: bei max_offene_mikrofone 3 wird das vierte
// Mikrofon abgelehnt und der Zustand bleibt unverändert.
func TestGrenzeOffenerMikrofone(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()

	// Die aktive Leitung darf fremde Plätze jederzeit schalten.
	for platz := 1; platz <= 3; platz++ {
		if err := p.kern.MikroAn(ctx, 1, platz); err != nil {
			t.Fatalf("mikro %d ließ sich nicht öffnen: %v", platz, err)
		}
	}
	vorher := p.kern.Zustand()

	err := p.kern.MikroAn(ctx, 1, 4)
	if code := codeVon(t, err); code != kern.CodeGrenzeErreicht {
		t.Fatalf("code %q erwartet, %q bekommen (%v)", kern.CodeGrenzeErreicht, code, err)
	}

	nachher := p.kern.Zustand()
	if nachher.Stand != vorher.Stand {
		t.Errorf("stand hat sich geändert: %d -> %d", vorher.Stand, nachher.Stand)
	}
	for i, pl := range nachher.Plaetze {
		if pl.Mikro != vorher.Plaetze[i].Mikro {
			t.Errorf("platz %d hat sich geändert", pl.Nummer)
		}
	}

	p.kern.KameraAbwarten()
	if abrufe := p.kamera.Abrufe(); len(abrufe) != 3 {
		t.Errorf("3 kameraabrufe erwartet, %d bekommen", len(abrufe))
	}

	if err := p.kern.MikroAus(ctx, 1, 1); err != nil {
		t.Fatalf("mikro 1 ließ sich nicht schließen: %v", err)
	}
	if err := p.kern.MikroAn(ctx, 1, 4); err != nil {
		t.Fatalf("mikro 4 nach dem freiwerden abgelehnt: %v", err)
	}
	p.kern.KameraAbwarten()
}

// TestPlatzUnbekannt: ein unbekannter Platz wird abgewiesen, nicht angelegt.
func TestPlatzUnbekannt(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)

	if code := codeVon(t, p.kern.MikroAn(context.Background(), 1, 99)); code != kern.CodePlatzUnbekannt {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodePlatzUnbekannt, code)
	}
	if anzahl := len(p.kern.Zustand().Plaetze); anzahl != 5 {
		t.Errorf("5 plätze erwartet, %d bekommen", anzahl)
	}
}

// TestPlatzBelegt: ein belegter Platz lässt sich kein zweites Mal anmelden.
func TestPlatzBelegt(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	ctx := context.Background()

	if err := p.kern.Anmelden(ctx, 2, "2222"); err != nil {
		t.Fatalf("anmelden fehlgeschlagen: %v", err)
	}
	if code := codeVon(t, p.kern.Anmelden(ctx, 2, "2222")); code != kern.CodePlatzBelegt {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodePlatzBelegt, code)
	}
}

// TestKameraausfallLaeuftWeiter: eine nicht erreichbare Kamera hält das
// System nicht auf, sie wird protokolliert.
func TestKameraausfallLaeuftWeiter(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	p.kamera.antwort = errors.New("keine antwort")
	eroeffnen(t, p)

	if err := p.kern.MikroAn(context.Background(), 1, 2); err != nil {
		t.Fatalf("mikro trotz kameraausfall abgelehnt: %v", err)
	}
	p.kern.KameraAbwarten()

	z := p.kern.Zustand()
	if !z.Plaetze[1].Mikro {
		t.Error("das mikrofon sollte offen sein")
	}
	if z.Kamera == nil || z.Kamera.Erreichbar {
		t.Errorf("kamera sollte als nicht erreichbar gemeldet sein, ist %+v", z.Kamera)
	}

	kette, err := p.ablage.Ereignisse(context.Background(), p.saalID)
	if err != nil {
		t.Fatalf("kette lesen: %v", err)
	}
	gefunden := false
	for _, e := range kette {
		if e.Art == "kamera_nicht_erreichbar" {
			gefunden = true
		}
	}
	if !gefunden {
		t.Error("ereignis kamera_nicht_erreichbar fehlt in der kette")
	}
}

// --- Meilenstein 2 ---

// TestPinFalsch: eine falsche PIN belegt den Platz nicht.
func TestPinFalsch(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())

	if code := codeVon(t, p.kern.Anmelden(context.Background(), 2, "0000")); code != kern.CodePinFalsch {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodePinFalsch, code)
	}
	if p.kern.Zustand().Plaetze[1].Belegt {
		t.Error("der platz darf nach falscher pin nicht belegt sein")
	}
}

// TestNurAktiveLeitungErteiltDasWort: weder ein Delegierter noch eine
// berechtigte, aber nicht aktive Leitung darf das Wort erteilen.
func TestNurAktiveLeitungErteiltDasWort(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()

	if err := p.kern.WortMelden(ctx, 2); err != nil {
		t.Fatalf("wortmeldung fehlgeschlagen: %v", err)
	}
	vorher := p.kern.Zustand()

	// Platz 5 ist Delegierter, Platz 4 ist zur Leitung berechtigt, führt aber
	// nicht — beide dürfen nicht erteilen.
	for _, absender := range []int{5, 4, 3} {
		if code := codeVon(t, p.kern.WortErteilen(ctx, absender, 2)); code != kern.CodeNichtBerechtigt {
			t.Errorf("platz %d: code %q erwartet, %q bekommen", absender, kern.CodeNichtBerechtigt, code)
		}
	}

	nachher := p.kern.Zustand()
	if nachher.Stand != vorher.Stand {
		t.Errorf("stand hat sich geändert: %d -> %d", vorher.Stand, nachher.Stand)
	}
	if nachher.Redeliste[0].Zustand != kern.WortGemeldet {
		t.Errorf("die wortmeldung sollte gemeldet bleiben, ist %s", nachher.Redeliste[0].Zustand)
	}

	if err := p.kern.WortErteilen(ctx, 1, 2); err != nil {
		t.Fatalf("die aktive leitung durfte nicht erteilen: %v", err)
	}
	if z := p.kern.Zustand(); z.Redeliste[0].Zustand != kern.WortErteilt {
		t.Errorf("erteilt erwartet, %s bekommen", z.Redeliste[0].Zustand)
	}
}

// TestStaffelstabGenauEineLeitung: nach der Übergabe darf die neue Leitung
// alles, die alte nichts mehr.
func TestStaffelstabGenauEineLeitung(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()

	if z := p.kern.Zustand(); z.Sitzung.LeitungPlatz != 1 {
		t.Fatalf("platz 1 sollte führen, führt aber %d", z.Sitzung.LeitungPlatz)
	}
	// An eine Rolle ohne Leitungsberechtigung wird nicht übergeben.
	if code := codeVon(t, p.kern.LeitungUebergeben(ctx, 1, 2)); code != kern.CodeNichtBerechtigt {
		t.Errorf("übergabe an einen delegierten hätte scheitern müssen, code %q", code)
	}

	if err := p.kern.LeitungUebergeben(ctx, 1, 4); err != nil {
		t.Fatalf("übergabe fehlgeschlagen: %v", err)
	}
	if z := p.kern.Zustand(); z.Sitzung.LeitungPlatz != 4 {
		t.Fatalf("platz 4 sollte führen, führt aber %d", z.Sitzung.LeitungPlatz)
	}

	if err := p.kern.WortMelden(ctx, 2); err != nil {
		t.Fatalf("wortmeldung fehlgeschlagen: %v", err)
	}
	// Die alte Leitung darf nicht mehr.
	if code := codeVon(t, p.kern.WortErteilen(ctx, 1, 2)); code != kern.CodeNichtBerechtigt {
		t.Errorf("die abgebende leitung darf nicht mehr erteilen, code %q", code)
	}
	// Die neue darf.
	if err := p.kern.WortErteilen(ctx, 4, 2); err != nil {
		t.Errorf("die neue leitung durfte nicht erteilen: %v", err)
	}

	// Die Übergabe steht in der Kette.
	platz, err := p.ablage.LeitungAusKette(ctx, p.saalID)
	if err != nil {
		t.Fatalf("kette lesen: %v", err)
	}
	if platz != 4 {
		t.Errorf("die kette sollte platz 4 nennen, nennt %d", platz)
	}
}

// TestMikroNurNachWorterteilung: ohne erteiltes Wort bleibt das Mikrofon zu.
func TestMikroNurNachWorterteilung(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	ctx := context.Background()

	// Ohne laufende Sitzung geht gar nichts.
	if code := codeVon(t, p.kern.MikroAn(ctx, 2, 2)); code != kern.CodeSitzungLaeuftNicht {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeSitzungLaeuftNicht, code)
	}
	eroeffnen(t, p)

	if code := codeVon(t, p.kern.MikroAn(ctx, 2, 2)); code != kern.CodeKeinWort {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeKeinWort, code)
	}
	if code := codeVon(t, p.kern.MikroAn(ctx, 2, 5)); code != kern.CodeNichtBerechtigt {
		t.Errorf("ein delegierter darf keinen fremden platz schalten, code %q", code)
	}

	// Die aktive Leitung darf jederzeit.
	if err := p.kern.MikroAn(ctx, 1, 1); err != nil {
		t.Fatalf("die leitung durfte nicht schalten: %v", err)
	}

	if err := p.kern.WortMelden(ctx, 2); err != nil {
		t.Fatalf("wortmeldung fehlgeschlagen: %v", err)
	}
	if err := p.kern.WortErteilen(ctx, 1, 2); err != nil {
		t.Fatalf("worterteilung fehlgeschlagen: %v", err)
	}
	if err := p.kern.MikroAn(ctx, 2, 2); err != nil {
		t.Fatalf("nach der worterteilung wurde abgelehnt: %v", err)
	}
	if z := p.kern.Zustand(); z.Redeliste[0].Zustand != kern.WortLaufend {
		t.Errorf("die wortmeldung sollte laufen, ist %s", z.Redeliste[0].Zustand)
	}

	// Entzug schließt das Mikrofon sofort.
	if err := p.kern.WortEntziehen(ctx, 1, 2); err != nil {
		t.Fatalf("wortentzug fehlgeschlagen: %v", err)
	}
	z := p.kern.Zustand()
	if z.Plaetze[1].Mikro {
		t.Error("das mikrofon sollte nach dem entzug zu sein")
	}
	if len(z.Redeliste) != 0 {
		t.Errorf("die redeliste sollte leer sein, hat %d einträge", len(z.Redeliste))
	}
	p.kern.KameraAbwarten()
}

// TestWortmeldungZustandskette: erlaubte Übergänge gehen, unerlaubte nicht.
func TestWortmeldungZustandskette(t *testing.T) {
	erlaubt := []struct{ von, nach kern.Wortzustand }{
		{kern.WortGemeldet, kern.WortErteilt},
		{kern.WortGemeldet, kern.WortZurueckgezogen},
		{kern.WortGemeldet, kern.WortEntzogen},
		{kern.WortErteilt, kern.WortLaufend},
		{kern.WortErteilt, kern.WortEntzogen},
		{kern.WortLaufend, kern.WortBeendet},
		{kern.WortLaufend, kern.WortEntzogen},
	}
	for _, u := range erlaubt {
		if !kern.WortUebergangErlaubt(u.von, u.nach) {
			t.Errorf("%s → %s sollte erlaubt sein", u.von, u.nach)
		}
	}

	verboten := []struct{ von, nach kern.Wortzustand }{
		{kern.WortBeendet, kern.WortLaufend},
		{kern.WortBeendet, kern.WortErteilt},
		{kern.WortEntzogen, kern.WortErteilt},
		{kern.WortZurueckgezogen, kern.WortGemeldet},
		{kern.WortLaufend, kern.WortGemeldet},
		{kern.WortErteilt, kern.WortGemeldet},
	}
	for _, u := range verboten {
		if kern.WortUebergangErlaubt(u.von, u.nach) {
			t.Errorf("%s → %s hätte verboten sein müssen", u.von, u.nach)
		}
	}

	// Dieselbe Regel über die Schnittstelle: eine beendete Wortmeldung steht
	// nicht mehr auf der Liste und lässt sich nicht wiederbeleben.
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()

	if err := p.kern.WortMelden(ctx, 2); err != nil {
		t.Fatalf("wortmeldung fehlgeschlagen: %v", err)
	}
	if err := p.kern.WortZurueckziehen(ctx, 2); err != nil {
		t.Fatalf("zurückziehen fehlgeschlagen: %v", err)
	}
	if len(p.kern.Zustand().Redeliste) != 0 {
		t.Error("die zurückgezogene meldung steht noch auf der liste")
	}
	if code := codeVon(t, p.kern.WortErteilen(ctx, 1, 2)); code != kern.CodeKeinWort {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeKeinWort, code)
	}
}

// TestDarfKommtAusDemKern: die Oberfläche bekommt die Rechte fertig geliefert.
func TestDarfKommtAusDemKern(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)

	leitung := p.kern.Ich(1)
	if leitung == nil {
		t.Fatal("die leitung sollte einen eigenen zustand haben")
	}
	if !enthaelt(leitung.Darf, kern.AktionWortErteilen) {
		t.Errorf("die aktive leitung sollte erteilen dürfen, darf: %v", leitung.Darf)
	}

	delegierter := p.kern.Ich(2)
	if enthaelt(delegierter.Darf, kern.AktionWortErteilen) {
		t.Errorf("ein delegierter darf nicht erteilen, darf: %v", delegierter.Darf)
	}
	if !enthaelt(delegierter.Darf, kern.AktionWortMelden) {
		t.Errorf("ein delegierter sollte sich melden dürfen, darf: %v", delegierter.Darf)
	}

	schrift := p.kern.Ich(3)
	if len(schrift.Darf) != 0 {
		t.Errorf("die schriftführung schaltet nichts, darf aber: %v", schrift.Darf)
	}

	zweiteLeitung := p.kern.Ich(4)
	if enthaelt(zweiteLeitung.Darf, kern.AktionWortErteilen) {
		t.Errorf("die nicht aktive leitung darf nicht erteilen, darf: %v", zweiteLeitung.Darf)
	}
}

func enthaelt(liste []string, wert string) bool {
	for _, e := range liste {
		if e == wert {
			return true
		}
	}
	return false
}

// TestZeitachseInJedemEreignis: jedes Ereignis nach der Eröffnung trägt die
// Sitzung und die Millisekunden seit Sitzungsbeginn. Ohne diese Zeitachse
// lassen sich Aufzeichnung, Transkript und Protokoll später nicht zusammen-
// bringen — nachrüsten geht nicht.
func TestZeitachseInJedemEreignis(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	ctx := context.Background()

	// Vor der Eröffnung gibt es keinen Nullpunkt, also auch keine Millisekunden.
	if err := p.kern.Anmelden(ctx, 2, "2222"); err != nil {
		t.Fatalf("anmelden: %v", err)
	}
	eroeffnen(t, p)

	if err := p.kern.WortMelden(ctx, 2); err != nil {
		t.Fatalf("wortmeldung: %v", err)
	}
	if err := p.kern.WortErteilen(ctx, 1, 2); err != nil {
		t.Fatalf("worterteilung: %v", err)
	}

	kette, err := p.ablage.Ereignisse(ctx, p.saalID)
	if err != nil {
		t.Fatalf("kette lesen: %v", err)
	}

	var vorher int64 = -1
	var mitZeit int
	for _, e := range kette {
		if e.Art == "platz_angemeldet" {
			if _, gefunden := e.Nutzlast["ms"]; gefunden {
				t.Error("vor der eröffnung darf es keine millisekunden geben")
			}
			continue
		}

		sitzung, gefunden := e.Nutzlast["sitzung"]
		if !gefunden || sitzung == "" {
			t.Errorf("ereignis %q ohne sitzung: %v", e.Art, e.Nutzlast)
		}
		roh, gefunden := e.Nutzlast["ms"]
		if !gefunden {
			t.Fatalf("ereignis %q ohne millisekunden: %v", e.Art, e.Nutzlast)
		}
		ms, passt := millisekunden(roh)
		if !passt {
			t.Fatalf("ereignis %q: millisekunden sind %T, nicht zählbar", e.Art, roh)
		}
		if ms < 0 {
			t.Errorf("ereignis %q liegt vor dem sitzungsbeginn: %d ms", e.Art, ms)
		}
		if ms < vorher {
			t.Errorf("die zeitachse läuft rückwärts: %d nach %d", ms, vorher)
		}
		vorher = ms
		mitZeit++
	}

	if mitZeit < 3 {
		t.Errorf("mindestens drei ereignisse auf der zeitachse erwartet, %d gefunden", mitZeit)
	}
	// Die Eröffnung selbst ist der Nullpunkt.
	if kette[1].Art != "sitzung_eroeffnet" {
		t.Fatalf("an stelle 2 wurde die eröffnung erwartet, %q gefunden", kette[1].Art)
	}
	if ms, _ := millisekunden(kette[1].Nutzlast["ms"]); ms > 50 {
		t.Errorf("die eröffnung sollte bei 0 ms liegen, liegt bei %d", ms)
	}
	if p.kern.Beginn().IsZero() {
		t.Error("der nullpunkt der zeitachse fehlt")
	}
}

// millisekunden liest den Wert unabhängig davon, ob er direkt aus dem Kern
// kommt (int64) oder den Umweg über jsonb genommen hat (float64).
func millisekunden(wert any) (int64, bool) {
	switch zahl := wert.(type) {
	case int64:
		return zahl, true
	case float64:
		return int64(zahl), true
	default:
		return 0, false
	}
}

// TestZeitachseImHash: die Millisekunden stehen in der Nutzlast und gehen
// damit in den Hash ein — eine nachträglich verschobene Zeitachse fällt auf.
func TestZeitachseImHash(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)

	kette, err := p.ablage.Ereignisse(context.Background(), p.saalID)
	if err != nil {
		t.Fatalf("kette lesen: %v", err)
	}
	if err := kern.KettePruefen(kette); err != nil {
		t.Fatalf("die kette sollte in ordnung sein: %v", err)
	}

	kette[0].Nutzlast["ms"] = int64(999999)
	if err := kern.KettePruefen(kette); err == nil {
		t.Fatal("eine verschobene zeitachse hätte auffallen müssen")
	}
}
