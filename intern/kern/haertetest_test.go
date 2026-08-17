package kern_test

import (
	"context"
	"sync"
	"testing"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
)

// Diese Tests suchen nicht die Bestätigung, dass alles läuft, sondern die
// Stellen, an denen es bricht. Was hier rot wird, ist ein Fund.

// TestLeitungVerschwindet: fällt das Gerät der Sitzungsleitung aus, muss der
// Saal handlungsfähig bleiben. Sonst steht die Sitzung, bis jemand den Server
// neu startet.
func TestLeitungVerschwindet(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4)
	eroeffnen(t, p)

	// Platz 1 führt und verlässt den Saal — das Gerät ist weg.
	if err := p.kern.Abmelden(ctx, 1); err != nil {
		t.Fatalf("abmelden: %v", err)
	}

	// Platz 4 ist ebenfalls zur Leitung berechtigt und noch da. Die Übernahme
	// ist eine eigene, ausdrückliche Handlung — das System vollzieht sie nie
	// von selbst.
	if err := p.kern.LeitungUebernehmen(ctx, 4); err != nil {
		t.Fatalf("die verbliebene leitung konnte nicht übernehmen: %v", err)
	}
	if z := p.kern.Zustand(); z.Sitzung.LeitungPlatz != 4 {
		t.Fatalf("platz 4 sollte führen, führt aber %d", z.Sitzung.LeitungPlatz)
	}
	if err := p.kern.SitzungSchliessen(ctx, 4); err != nil {
		t.Errorf("die neue leitung konnte die sitzung nicht schließen: %v", err)
	}
}

// TestUebernahmeNurBeiVerwaistemPlatz: solange die Leitung besetzt ist, wird
// übergeben und nicht übernommen. Sonst wäre der Staffelstab keiner.
func TestUebernahmeNurBeiVerwaistemPlatz(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 4)
	eroeffnen(t, p)

	if code := codeVon(t, p.kern.LeitungUebernehmen(ctx, 4)); code != kern.CodeNichtBerechtigt {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeNichtBerechtigt, code)
	}
	if z := p.kern.Zustand(); z.Sitzung.LeitungPlatz != 1 {
		t.Errorf("platz 1 sollte weiter führen, führt aber %d", z.Sitzung.LeitungPlatz)
	}
	// Und ein Delegierter übernimmt nie.
	if err := p.kern.Abmelden(ctx, 1); err != nil {
		t.Fatalf("abmelden: %v", err)
	}
	anmeldenAlle(t, p, 2)
	if code := codeVon(t, p.kern.LeitungUebernehmen(ctx, 2)); code != kern.CodeNichtBerechtigt {
		t.Errorf("ein delegierter darf nicht übernehmen, code %q", code)
	}
}

// TestSitzungSchliessenWaehrendAbstimmung: eine laufende Abstimmung darf nicht
// als laufend zurückbleiben, wenn die Sitzung endet.
func TestSitzungSchliessenWaehrendAbstimmung(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4)
	eroeffnen(t, p)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Haushalt", kern.AbstimmungOffen); err != nil {
		t.Fatalf("starten: %v", err)
	}
	if err := p.kern.StimmeAbgeben(ctx, 1, kern.WahlJa); err != nil {
		t.Fatalf("stimme: %v", err)
	}
	if err := p.kern.SitzungSchliessen(ctx, 1); err != nil {
		t.Fatalf("schließen: %v", err)
	}

	z := p.kern.Zustand()
	if z.Abstimmung != nil && z.Abstimmung.Zustand == kern.AbstimmungLaufend {
		t.Fatal("die abstimmung steht nach dem sitzungsende noch auf laufend — " +
			"sie lässt sich nie mehr beenden")
	}
}

// TestWortmeldungBleibtNichtVerwaist: wer den Saal verlässt, verschwindet auch
// aus der Redeliste. Sonst erteilt die Leitung das Wort an einen leeren Platz.
func TestWortmeldungBleibtNichtVerwaist(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2)
	eroeffnen(t, p)

	if err := p.kern.WortMelden(ctx, 2); err != nil {
		t.Fatalf("wortmeldung: %v", err)
	}
	if err := p.kern.Abmelden(ctx, 2); err != nil {
		t.Fatalf("abmelden: %v", err)
	}

	if z := p.kern.Zustand(); len(z.Redeliste) != 0 {
		t.Fatalf("die redeliste enthält noch die meldung eines leeren platzes: %+v", z.Redeliste)
	}
}

// TestGleichzeitigeStimmen: viele Stimmen zur selben Zeit dürfen weder doppelt
// zählen noch verloren gehen.
func TestGleichzeitigeStimmen(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)
	eroeffnen(t, p)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Haushalt", kern.AbstimmungOffen); err != nil {
		t.Fatalf("starten: %v", err)
	}

	// Jeder Platz stimmt gleichzeitig, und zwar mehrfach.
	var warten sync.WaitGroup
	for _, platz := range []int{1, 2, 4, 5} {
		for versuch := 0; versuch < 5; versuch++ {
			warten.Add(1)
			go func(platz int) {
				defer warten.Done()
				_ = p.kern.StimmeAbgeben(ctx, platz, kern.WahlJa)
			}(platz)
		}
	}
	warten.Wait()

	if err := p.kern.AbstimmungBeenden(ctx, 1); err != nil {
		t.Fatalf("auszählen: %v", err)
	}
	e := p.kern.Zustand().Abstimmung.Ergebnis
	if e.Ja != 4 || e.Nein != 0 || e.Enthaltung != 0 {
		t.Errorf("genau 4 stimmen erwartet, gezählt wurden %d ja, %d nein, %d enthaltungen",
			e.Ja, e.Nein, e.Enthaltung)
	}
}

// TestGleichzeitigeMikrofone: die Grenze hält auch unter Last.
func TestGleichzeitigeMikrofone(t *testing.T) {
	p := aufbauen(t, 5, 2, standardbesetzung())
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4, 5)
	eroeffnen(t, p)

	var warten sync.WaitGroup
	for _, platz := range []int{1, 2, 4, 5} {
		warten.Add(1)
		go func(platz int) {
			defer warten.Done()
			_ = p.kern.MikroAn(ctx, 1, platz)
		}(platz)
	}
	warten.Wait()
	p.kern.KameraAbwarten()

	offen := 0
	for _, pl := range p.kern.Zustand().Plaetze {
		if pl.Mikro {
			offen++
		}
	}
	if offen > 2 {
		t.Errorf("höchstens 2 mikrofone erlaubt, %d sind offen", offen)
	}
}

// TestNeustartMittenInDerAbstimmung: stirbt der Server während einer
// Abstimmung, darf beim Neustart nichts verloren gehen — weder die Zählung
// noch die Information, wer schon abgestimmt hat.
func TestNeustartMittenInDerAbstimmung(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4)
	eroeffnen(t, p)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Haushalt", kern.AbstimmungGeheim); err != nil {
		t.Fatalf("starten: %v", err)
	}
	if err := p.kern.StimmeAbgeben(ctx, 1, kern.WahlJa); err != nil {
		t.Fatalf("stimme: %v", err)
	}
	if err := p.kern.StimmeAbgeben(ctx, 2, kern.WahlNein); err != nil {
		t.Fatalf("stimme: %v", err)
	}
	vorher := p.kern.Zustand()

	// Der Server startet neu: derselbe Zustand wird aus der Ablage geholt.
	nachher := neustarten(t, p)

	z := nachher.Zustand()
	if z.Abstimmung == nil {
		t.Fatal("die abstimmung ist beim neustart verloren gegangen")
	}
	if z.Abstimmung.Zustand != kern.AbstimmungLaufend {
		t.Errorf("laufend erwartet, %s bekommen", z.Abstimmung.Zustand)
	}
	if z.Abstimmung.Abgegeben != vorher.Abstimmung.Abgegeben {
		t.Errorf("%d abgegebene stimmen vor, %d nach dem neustart",
			vorher.Abstimmung.Abgegeben, z.Abstimmung.Abgegeben)
	}
	if z.Abstimmung.Quorum != vorher.Abstimmung.Quorum || z.Abstimmung.Anwesend != vorher.Abstimmung.Anwesend {
		t.Errorf("die eingefrorene beschlussfähigkeit hat sich geändert: %+v", z.Abstimmung)
	}

	// Wer schon abgestimmt hat, kann es auch nach dem Neustart nicht erneut.
	if code := codeVon(t, nachher.StimmeAbgeben(ctx, 1, kern.WahlJa)); code != kern.CodeSchonAbgestimmt {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeSchonAbgestimmt, code)
	}
	// Und die Zählung stimmt am Ende trotzdem.
	if err := nachher.StimmeAbgeben(ctx, 4, kern.WahlJa); err != nil {
		t.Fatalf("dritte stimme: %v", err)
	}
	if err := nachher.AbstimmungBeenden(ctx, 1); err != nil {
		t.Fatalf("auszählen: %v", err)
	}
	if e := nachher.Zustand().Abstimmung.Ergebnis; e.Ja != 2 || e.Nein != 1 {
		t.Errorf("2 ja, 1 nein erwartet: %+v", e)
	}
}
