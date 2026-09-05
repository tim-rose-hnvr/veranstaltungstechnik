package kern_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
)

// anmeldenAlle bringt die genannten Plätze in den Saal.
func anmeldenAlle(t *testing.T, p *pruefstand, plaetze ...int) {
	t.Helper()
	pins := map[int]string{1: "1111", 2: "2222", 3: "3333", 4: "4444", 5: "5555"}
	for _, platz := range plaetze {
		if err := p.kern.Anmelden(context.Background(), platz, pins[platz]); err != nil {
			t.Fatalf("platz %d anmelden: %v", platz, err)
		}
	}
}

// TestAbstimmungNurBeiBeschlussfaehigkeit: ohne Quorum wird gar nicht erst
// gestartet, und das Quorum wird beim Start eingefroren.
func TestAbstimmungNurBeiBeschlussfaehigkeit(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()

	// Stimmberechtigt sind Plätze 1, 2, 4, 5 (leitung und delegierter).
	// Quorum ist damit 3. Nur zwei anwesend: zu wenig.
	anmeldenAlle(t, p, 1, 2)
	err := p.kern.AbstimmungStarten(ctx, 1, "Beschluss über den Haushalt", kern.AbstimmungOffen)
	if code := codeVon(t, err); code != kern.CodeNichtBeschlussfaehig {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeNichtBeschlussfaehig, code)
	}
	var f *kern.Fehler
	if ok := errorsAs(err, &f); !ok || !strings.Contains(f.Text, "nötig sind 3") {
		t.Errorf("der text sollte das quorum nennen: %v", err)
	}
	if p.kern.Zustand().Abstimmung != nil {
		t.Error("es darf keine abstimmung angelegt worden sein")
	}

	anmeldenAlle(t, p, 4)
	if err := p.kern.AbstimmungStarten(ctx, 1, "Beschluss über den Haushalt", kern.AbstimmungOffen); err != nil {
		t.Fatalf("abstimmung ließ sich nicht starten: %v", err)
	}

	z := p.kern.Zustand().Abstimmung
	if z == nil {
		t.Fatal("die abstimmung fehlt im zustand")
	}
	if z.Stimmberechtigt != 4 || z.Anwesend != 3 || z.Quorum != 3 {
		t.Errorf("stimmberechtigt 4, anwesend 3, quorum 3 erwartet: %+v", z)
	}

	// Wer später kommt, ändert die eingefrorene Beschlussfähigkeit nicht.
	anmeldenAlle(t, p, 5)
	if nachher := p.kern.Zustand().Abstimmung; nachher.Anwesend != 3 {
		t.Errorf("die beschlussfähigkeit war eingefroren, ist jetzt %d", nachher.Anwesend)
	}
}

// TestGeheimeWahlOhneZuordnung: bei geheimer Wahl darf die Zuordnung Stimme
// zu Person nirgends existieren — auch nicht im Ereignisprotokoll.
func TestGeheimeWahlOhneZuordnung(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Wahl der Vorsitzenden", kern.AbstimmungGeheim); err != nil {
		t.Fatalf("abstimmung starten: %v", err)
	}
	stimmen := map[int]kern.Wahl{1: kern.WahlJa, 2: kern.WahlJa, 4: kern.WahlNein, 5: kern.WahlEnthaltung}
	for platz, wahl := range stimmen {
		if err := p.kern.StimmeAbgeben(ctx, platz, wahl, "", ""); err != nil {
			t.Fatalf("platz %d: %v", platz, err)
		}
	}

	kette, err := p.ablage.Ereignisse(ctx, p.saalID)
	if err != nil {
		t.Fatalf("kette lesen: %v", err)
	}
	abgegeben := 0
	for _, e := range kette {
		if e.Art != "stimme_abgegeben" {
			continue
		}
		abgegeben++
		if _, gefunden := e.Nutzlast["platz"]; gefunden {
			t.Errorf("geheime wahl: das ereignis nennt den platz: %v", e.Nutzlast)
		}
		if _, gefunden := e.Nutzlast["wahl"]; gefunden {
			t.Errorf("geheime wahl: das ereignis nennt die stimme: %v", e.Nutzlast)
		}
	}
	if abgegeben != 4 {
		t.Errorf("4 stimmereignisse erwartet, %d gefunden", abgegeben)
	}

	// Der Zustand verrät während der Wahl nur, wie viele abgestimmt haben.
	z := p.kern.Zustand().Abstimmung
	if z.Abgegeben != 4 {
		t.Errorf("4 abgegebene stimmen erwartet, %d gemeldet", z.Abgegeben)
	}
	if z.Ergebnis != nil {
		t.Error("vor dem auszählen darf es kein ergebnis geben")
	}

	if err := p.kern.AbstimmungBeenden(ctx, 1); err != nil {
		t.Fatalf("auszählen: %v", err)
	}
	z = p.kern.Zustand().Abstimmung
	if z.Ergebnis == nil {
		t.Fatal("nach dem auszählen fehlt das ergebnis")
	}
	if z.Ergebnis.Ja != 2 || z.Ergebnis.Nein != 1 || z.Ergebnis.Enthaltung != 1 {
		t.Errorf("2/1/1 erwartet: %+v", z.Ergebnis)
	}
	if !z.Ergebnis.Angenommen {
		t.Error("2 ja gegen 1 nein ist angenommen")
	}
	if len(z.Ergebnis.Namentlich) != 0 {
		t.Errorf("geheime wahl darf keine namentliche liste liefern: %v", z.Ergebnis.Namentlich)
	}
}

// TestZwischenstandBleibtVerborgen: solange abgestimmt wird, ist die Zählung
// unsichtbar — ein Zwischenstand beeinflusst die ausstehenden Stimmen.
func TestZwischenstandBleibtVerborgen(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Beschluss", kern.AbstimmungOffen); err != nil {
		t.Fatalf("starten: %v", err)
	}
	if err := p.kern.StimmeAbgeben(ctx, 1, kern.WahlJa, "", ""); err != nil {
		t.Fatalf("stimme: %v", err)
	}

	z := p.kern.Zustand().Abstimmung
	if z.Ergebnis != nil {
		t.Fatalf("während der abstimmung darf kein zwischenstand sichtbar sein: %+v", z.Ergebnis)
	}
	if z.Abgegeben != 1 {
		t.Errorf("eine abgegebene stimme erwartet, %d gemeldet", z.Abgegeben)
	}
}

// TestDoppelteStimmeAbgelehnt: zweimal abstimmen geht nicht, auch nicht
// bei geheimer Wahl.
func TestDoppelteStimmeAbgelehnt(t *testing.T) {
	for _, art := range []kern.Abstimmungsart{kern.AbstimmungOffen, kern.AbstimmungGeheim} {
		t.Run(string(art), func(t *testing.T) {
			p := aufbauen(t, 5, 3, standardbesetzung())
			eroeffnen(t, p)
			ctx := context.Background()
			anmeldenAlle(t, p, 1, 2, 4, 5)

			if err := p.kern.AbstimmungStarten(ctx, 1, "Beschluss", art); err != nil {
				t.Fatalf("starten: %v", err)
			}
			if err := p.kern.StimmeAbgeben(ctx, 2, kern.WahlJa, "", ""); err != nil {
				t.Fatalf("erste stimme: %v", err)
			}
			if code := codeVon(t, p.kern.StimmeAbgeben(ctx, 2, kern.WahlNein, "", "")); code != kern.CodeSchonAbgestimmt {
				t.Fatalf("code %q erwartet, %q bekommen", kern.CodeSchonAbgestimmt, code)
			}
			if err := p.kern.AbstimmungBeenden(ctx, 1); err != nil {
				t.Fatalf("auszählen: %v", err)
			}
			if e := p.kern.Zustand().Abstimmung.Ergebnis; e.Ja != 1 || e.Nein != 0 {
				t.Errorf("die zweite stimme darf nicht gezählt worden sein: %+v", e)
			}
		})
	}
}

// TestOhneStimmrechtKeineStimme: die Schriftführung stimmt nicht ab.
func TestOhneStimmrechtKeineStimme(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 3, 4)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Beschluss", kern.AbstimmungOffen); err != nil {
		t.Fatalf("starten: %v", err)
	}
	if code := codeVon(t, p.kern.StimmeAbgeben(ctx, 3, kern.WahlJa, "", "")); code != kern.CodeNichtBerechtigt {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeNichtBerechtigt, code)
	}
	// Und ein Delegierter startet keine Abstimmung.
	if code := codeVon(t, p.kern.AbstimmungStarten(ctx, 2, "Meine", kern.AbstimmungOffen)); code != kern.CodeNichtBerechtigt {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeNichtBerechtigt, code)
	}
}

// TestKeineTechnikeingriffeWaehrendDerAbstimmung: das Mikrofon geht auf, die
// Kamera bleibt stehen. Eine Kamerafahrt mitten in der Abstimmung ist genau
// der Automatismus, der einen Kunden kostet.
func TestKeineTechnikeingriffeWaehrendDerAbstimmung(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)

	if err := p.kern.MikroAn(ctx, 1, 1); err != nil {
		t.Fatalf("mikro vor der abstimmung: %v", err)
	}
	p.kern.KameraAbwarten()
	vorher := len(p.kamera.Abrufe())
	if vorher != 1 {
		t.Fatalf("ein kameraabruf erwartet, %d bekommen", vorher)
	}

	if err := p.kern.AbstimmungStarten(ctx, 1, "Beschluss", kern.AbstimmungOffen); err != nil {
		t.Fatalf("starten: %v", err)
	}
	if err := p.kern.MikroAn(ctx, 1, 2); err != nil {
		t.Fatalf("mikro während der abstimmung: %v", err)
	}
	p.kern.KameraAbwarten()

	if nachher := len(p.kamera.Abrufe()); nachher != vorher {
		t.Errorf("die kamera darf während der abstimmung nicht fahren: %d abrufe", nachher)
	}
	if !p.kern.Zustand().Plaetze[1].Mikro {
		t.Error("das mikrofon sollte trotzdem offen sein")
	}

	// Nach dem Auszählen fährt sie wieder.
	if err := p.kern.AbstimmungBeenden(ctx, 1); err != nil {
		t.Fatalf("auszählen: %v", err)
	}
	if err := p.kern.MikroAn(ctx, 1, 4); err != nil {
		t.Fatalf("mikro nach der abstimmung: %v", err)
	}
	p.kern.KameraAbwarten()
	if len(p.kamera.Abrufe()) != vorher+1 {
		t.Error("nach der abstimmung sollte die kamera wieder fahren")
	}
}

// TestAbstimmungZustandskette: erst auszählen, dann feststellen.
func TestAbstimmungZustandskette(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)

	// Ohne laufende Abstimmung geht nichts.
	if code := codeVon(t, p.kern.StimmeAbgeben(ctx, 2, kern.WahlJa, "", "")); code != kern.CodeKeineAbstimmung {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeKeineAbstimmung, code)
	}
	if err := p.kern.AbstimmungStarten(ctx, 1, "Beschluss", kern.AbstimmungNamentlich); err != nil {
		t.Fatalf("starten: %v", err)
	}
	// Zwei Abstimmungen gleichzeitig gibt es nicht.
	if code := codeVon(t, p.kern.AbstimmungStarten(ctx, 1, "Zweite", kern.AbstimmungOffen)); code != kern.CodeAbstimmungLaeuft {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeAbstimmungLaeuft, code)
	}
	// Feststellen vor dem Auszählen geht nicht.
	if code := codeVon(t, p.kern.AbstimmungFeststellen(ctx, 1)); code != kern.CodeKeineAbstimmung {
		t.Errorf("feststellen vor dem auszählen hätte scheitern müssen, code %q", code)
	}

	if err := p.kern.StimmeAbgeben(ctx, 2, kern.WahlNein, "", ""); err != nil {
		t.Fatalf("stimme: %v", err)
	}
	if err := p.kern.AbstimmungBeenden(ctx, 1); err != nil {
		t.Fatalf("auszählen: %v", err)
	}
	if err := p.kern.AbstimmungFeststellen(ctx, 1); err != nil {
		t.Fatalf("feststellen: %v", err)
	}

	z := p.kern.Zustand().Abstimmung
	if z.Zustand != kern.AbstimmungFestgestellt {
		t.Errorf("festgestellt erwartet, %s bekommen", z.Zustand)
	}
	if z.Ergebnis.Angenommen {
		t.Error("0 ja gegen 1 nein ist abgelehnt")
	}
	// Namentlich: die Zuordnung ist gewollt und wird ausgewiesen.
	if z.Ergebnis.Namentlich[2] != kern.WahlNein {
		t.Errorf("namentliche zuordnung fehlt: %v", z.Ergebnis.Namentlich)
	}
}

// errorsAs kapselt errors.As für die Lesbarkeit oben.
func errorsAs(err error, ziel **kern.Fehler) bool {
	f, ok := err.(*kern.Fehler)
	if ok {
		*ziel = f
	}
	return ok
}

// TestGepufferteStimmeDarfWiederholtWerden: dieselbe Marke noch einmal ist
// dieselbe Stimme — Erfolg, aber nur einmal gezählt. Eine fremde Marke ist
// ein Doppelversuch und wird abgewiesen. Darauf steht der Offline-Puffer:
// das Gerät darf nach einem Verbindungsabriss blind nachliefern.
func TestGepufferteStimmeDarfWiederholtWerden(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Beschluss", kern.AbstimmungOffen); err != nil {
		t.Fatalf("abstimmung starten: %v", err)
	}
	id := p.kern.Zustand().Abstimmung.ID
	if id == "" {
		t.Fatal("die abstimmung braucht eine kennung im zustand, sonst kann kein gerät puffern")
	}

	if err := p.kern.StimmeAbgeben(ctx, 2, kern.WahlJa, "marke-eins", id); err != nil {
		t.Fatalf("erste abgabe: %v", err)
	}
	// Die Wiederholung mit derselben Marke: Erfolg, keine zweite Stimme.
	if err := p.kern.StimmeAbgeben(ctx, 2, kern.WahlJa, "marke-eins", id); err != nil {
		t.Fatalf("die wiederholung der eigenen stimme muss erfolg melden: %v", err)
	}
	// Eine andere Marke vom selben Platz ist ein Doppelversuch.
	if code := codeVon(t, p.kern.StimmeAbgeben(ctx, 2, kern.WahlNein, "marke-zwei", id)); code != kern.CodeSchonAbgestimmt {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeSchonAbgestimmt, code)
	}

	if err := p.kern.AbstimmungBeenden(ctx, 1); err != nil {
		t.Fatalf("auszählen: %v", err)
	}
	e := p.kern.Zustand().Abstimmung.Ergebnis
	if e.Ja != 1 || e.Nein != 0 {
		t.Errorf("genau eine ja-stimme erwartet, ja=%d nein=%d", e.Ja, e.Nein)
	}
}

// TestVerspaeteteStimmeVerfaellt: eine gepufferte Stimme für eine Abstimmung,
// die inzwischen vorbei ist, verfällt mit klarer Ansage — sie darf vor allem
// nicht in der nächsten Abstimmung mitgezählt werden.
func TestVerspaeteteStimmeVerfaellt(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Erster Beschluss", kern.AbstimmungOffen); err != nil {
		t.Fatalf("abstimmung starten: %v", err)
	}
	alteID := p.kern.Zustand().Abstimmung.ID
	if err := p.kern.StimmeAbgeben(ctx, 1, kern.WahlJa, "", ""); err != nil {
		t.Fatalf("stimme der leitung: %v", err)
	}
	if err := p.kern.AbstimmungBeenden(ctx, 1); err != nil {
		t.Fatalf("auszählen: %v", err)
	}

	// Nach dem Auszählen: verfallen, nicht "schon abgestimmt".
	if code := codeVon(t, p.kern.StimmeAbgeben(ctx, 2, kern.WahlJa, "m", alteID)); code != kern.CodeStimmeVerfallen {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeStimmeVerfallen, code)
	}

	// Und in der nächsten Abstimmung erst recht: die alte Kennung passt nicht.
	if err := p.kern.AbstimmungStarten(ctx, 1, "Zweiter Beschluss", kern.AbstimmungOffen); err != nil {
		t.Fatalf("zweite abstimmung: %v", err)
	}
	if code := codeVon(t, p.kern.StimmeAbgeben(ctx, 2, kern.WahlJa, "m", alteID)); code != kern.CodeStimmeVerfallen {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeStimmeVerfallen, code)
	}
	if abgegeben := p.kern.Zustand().Abstimmung.Abgegeben; abgegeben != 0 {
		t.Errorf("die verfallene stimme wurde mitgezählt: %d abgegeben", abgegeben)
	}
	// Ohne Kennung gilt das alte Verhalten: die Stimme zählt für die laufende.
	if err := p.kern.StimmeAbgeben(ctx, 2, kern.WahlJa, "", ""); err != nil {
		t.Fatalf("stimme ohne bindung: %v", err)
	}
}

// TestMarkeBleibtAusDerKette: die Marke ist ein Gerätewert und hat im
// Ereignisprotokoll nichts verloren — bei geheimer Wahl stünde sonst ein
// Bindeglied zur Person in der Kette.
func TestMarkeBleibtAusDerKette(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Wahl", kern.AbstimmungGeheim); err != nil {
		t.Fatalf("abstimmung starten: %v", err)
	}
	id := p.kern.Zustand().Abstimmung.ID
	if err := p.kern.StimmeAbgeben(ctx, 2, kern.WahlJa, "geraete-marke", id); err != nil {
		t.Fatalf("stimme: %v", err)
	}

	kette, err := p.ablage.Ereignisse(ctx, p.saalID)
	if err != nil {
		t.Fatalf("kette lesen: %v", err)
	}
	for _, e := range kette {
		if _, da := e.Nutzlast["marke"]; da {
			t.Fatalf("die marke steht in der kette: %v", e.Nutzlast)
		}
	}
}

// TestGepufferteStimmeNachNeustart: stürzt der Server mitten in der
// Abstimmung ab, kommt "wer hat schon abgestimmt" aus der Ablage zurück —
// die Marken aber nicht, sie leben nur im Arbeitsspeicher. Ein Gerät, das
// seine Stimme nachreichert, bekommt dann "schon abgestimmt" statt der
// Bestätigung. Das ist die ehrliche Antwort: gezählt ist gezählt, doppelt
// wird nie.
func TestGepufferteStimmeNachNeustart(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Beschluss", kern.AbstimmungOffen); err != nil {
		t.Fatalf("abstimmung starten: %v", err)
	}
	id := p.kern.Zustand().Abstimmung.ID
	// Die Stimme kommt an, aber die Bestätigung erreicht das Gerät nie —
	// genau dann bleibt der Puffer stehen und wird später nachgereicht.
	if err := p.kern.StimmeAbgeben(ctx, 2, kern.WahlJa, "marke-vor-dem-sturz", id); err != nil {
		t.Fatalf("stimme: %v", err)
	}

	nachher := neustarten(t, p)

	z := nachher.Zustand().Abstimmung
	if z == nil || z.Zustand != kern.AbstimmungLaufend {
		t.Fatalf("die laufende abstimmung muss den neustart überleben: %+v", z)
	}
	if z.Abgegeben != 1 {
		t.Fatalf("eine abgegebene stimme erwartet, %d bekommen", z.Abgegeben)
	}

	// Das Gerät reicht nach: dieselbe Marke, aber der Server kennt sie nicht
	// mehr. Die Antwort ist "schon abgestimmt" — das Gerät räumt den Puffer.
	if code := codeVon(t, nachher.StimmeAbgeben(ctx, 2, kern.WahlJa, "marke-vor-dem-sturz", id)); code != kern.CodeSchonAbgestimmt {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeSchonAbgestimmt, code)
	}
	if z := nachher.Zustand().Abstimmung; z.Abgegeben != 1 {
		t.Errorf("die nachgereichte stimme wurde doppelt gezählt: %d", z.Abgegeben)
	}
}

// TestStimmeNachNeustartVonNeuemPlatz: der Fall, den der Neustart-Test oben
// verfehlt hat. Dort stimmte ein Platz nach, der schon abgestimmt hatte —
// die Prüfung "schon abgestimmt" griff vor der Stelle, die brach.
//
// Hier stimmt ein Platz ab, der VORHER noch nicht abgestimmt hatte, und zwar
// mit Marke, so wie jedes Gerät es tut. Marken wird von keiner Ablage
// wiederhergestellt — die Karte kam als nil zurück, und der Schreibversuch
// riss den Kern samt Sperre mit sich: der Saal fror ein und jede weitere
// Stimme war verloren. Genau das darf nie wieder passieren.
func TestStimmeNachNeustartVonNeuemPlatz(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	eroeffnen(t, p)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Beschluss", kern.AbstimmungOffen); err != nil {
		t.Fatalf("abstimmung starten: %v", err)
	}
	id := p.kern.Zustand().Abstimmung.ID
	if err := p.kern.StimmeAbgeben(ctx, 1, kern.WahlJa, "marke-eins", id); err != nil {
		t.Fatalf("erste stimme: %v", err)
	}

	nachher := neustarten(t, p)

	// Platz 2 hatte noch nicht abgestimmt und reicht jetzt mit Marke nach.
	if err := nachher.StimmeAbgeben(ctx, 2, kern.WahlNein, "marke-zwei", id); err != nil {
		t.Fatalf("stimme nach dem neustart: %v", err)
	}
	// Und die Wiederholung derselben Marke muss weiterhin bestätigt werden,
	// sonst hätte der Neustart den Offline-Puffer stillschweigend entwertet.
	if err := nachher.StimmeAbgeben(ctx, 2, kern.WahlNein, "marke-zwei", id); err != nil {
		t.Fatalf("wiederholung nach dem neustart: %v", err)
	}

	// Der Kern muss weiter ansprechbar sein — eine stehengebliebene Sperre
	// würde hier hängen, nicht fehlschlagen.
	if err := nachher.StimmeAbgeben(ctx, 4, kern.WahlJa, "marke-vier", id); err != nil {
		t.Fatalf("dritte stimme: %v", err)
	}
	if err := nachher.AbstimmungBeenden(ctx, 1); err != nil {
		t.Fatalf("auszählen: %v", err)
	}
	e := nachher.Zustand().Abstimmung.Ergebnis
	if e.Ja != 2 || e.Nein != 1 {
		t.Errorf("ja=2 nein=1 erwartet, ja=%d nein=%d enthaltung=%d", e.Ja, e.Nein, e.Enthaltung)
	}
}
