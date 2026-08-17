package kern_test

import (
	"context"
	"testing"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
)

func nein() *bool { falsch := false; return &falsch }

// tagesordnung: drei Punkte, der mittlere nicht öffentlich.
func tagesordnung() []speicher.Topdaten {
	return []speicher.Topdaten{
		{Nummer: 1, Titel: "Begrüßung und Feststellung der Beschlussfähigkeit"},
		{Nummer: 2, Titel: "Personalangelegenheit", Oeffentlich: nein()},
		{Nummer: 3, Titel: "Jahresabschluss"},
	}
}

func mitTagesordnung(t *testing.T) *pruefstand {
	t.Helper()
	p := aufbauen(t, 5, 3, standardbesetzung(), tagesordnung()...)
	anmeldenAlle(t, p, 1, 2, 4, 5)
	eroeffnen(t, p)
	return p
}

// TestPunktAufrufenSchliesstDenVorigen: es ist immer höchstens einer in Arbeit.
func TestPunktAufrufenSchliesstDenVorigen(t *testing.T) {
	p := mitTagesordnung(t)
	ctx := context.Background()

	if err := p.kern.TopAufrufen(ctx, 1, 1); err != nil {
		t.Fatalf("punkt 1 aufrufen: %v", err)
	}
	if err := p.kern.TopAufrufen(ctx, 1, 3); err != nil {
		t.Fatalf("punkt 3 aufrufen: %v", err)
	}

	z := p.kern.Zustand()
	if z.Sitzung.AktuellerTop != 3 {
		t.Errorf("punkt 3 sollte laufen, es läuft %d", z.Sitzung.AktuellerTop)
	}
	laufend := 0
	for _, top := range z.Tagesordnung {
		if top.Zustand == kern.TopLaufend {
			laufend++
		}
		if top.Nummer == 1 && top.Zustand != kern.TopAbgeschlossen {
			t.Errorf("punkt 1 sollte abgeschlossen sein, ist %s", top.Zustand)
		}
	}
	if laufend != 1 {
		t.Errorf("genau ein punkt darf laufen, es sind %d", laufend)
	}
}

// TestNichtOeffentlicherPunktPausiertDieAufzeichnung: das entscheidet der
// Sitzungszustand, nicht ein Handgriff an der Technik.
func TestNichtOeffentlicherPunktPausiertDieAufzeichnung(t *testing.T) {
	p := mitTagesordnung(t)
	ctx := context.Background()

	if err := p.kern.TopAufrufen(ctx, 1, 1); err != nil {
		t.Fatalf("punkt 1: %v", err)
	}
	if !p.kern.Zustand().Aufzeichnung {
		t.Fatal("bei einem öffentlichen punkt läuft die aufzeichnung")
	}

	if err := p.kern.TopAufrufen(ctx, 1, 2); err != nil {
		t.Fatalf("punkt 2: %v", err)
	}
	if p.kern.Zustand().Aufzeichnung {
		t.Fatal("beim nicht öffentlichen punkt muss die aufzeichnung pausieren")
	}
	if !hatEreignis(t, p, "aufzeichnung_pausiert") {
		t.Error("die pause steht nicht im protokoll")
	}

	if err := p.kern.TopAufrufen(ctx, 1, 3); err != nil {
		t.Fatalf("punkt 3: %v", err)
	}
	if !p.kern.Zustand().Aufzeichnung {
		t.Error("nach dem nicht öffentlichen punkt läuft die aufzeichnung wieder")
	}
	if !hatEreignis(t, p, "aufzeichnung_fortgesetzt") {
		t.Error("die fortsetzung steht nicht im protokoll")
	}
}

// TestBeschlussBrauchtEinenPunkt: ein Beschluss ohne Tagesordnungspunkt ist
// später nicht mehr zuzuordnen.
func TestBeschlussBrauchtEinenPunkt(t *testing.T) {
	p := mitTagesordnung(t)
	ctx := context.Background()

	code := codeVon(t, p.kern.AbstimmungStarten(ctx, 1, "Haushalt", kern.AbstimmungOffen))
	if code != kern.CodeKeinTop {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeKeinTop, code)
	}

	if err := p.kern.TopAufrufen(ctx, 1, 3); err != nil {
		t.Fatalf("punkt 3: %v", err)
	}
	if err := p.kern.AbstimmungStarten(ctx, 1, "Haushalt", kern.AbstimmungOffen); err != nil {
		t.Fatalf("abstimmung unter punkt 3: %v", err)
	}

	// Und der Beschluss trägt den Punkt, unter dem er gefasst wurde.
	kette, err := p.ablage.Ereignisse(ctx, p.saalID)
	if err != nil {
		t.Fatalf("kette lesen: %v", err)
	}
	gefunden := false
	for _, e := range kette {
		if e.Art != "abstimmung_gestartet" {
			continue
		}
		gefunden = true
		if top, dabei := e.Nutzlast["top"]; !dabei || zahlAus(top) != 3 {
			t.Errorf("der beschluss trägt den punkt nicht: %+v", e.Nutzlast)
		}
	}
	if !gefunden {
		t.Error("kein abstimmung_gestartet in der kette")
	}
}

// TestPunktWechseltNichtWaehrendDerAbstimmung: sonst bliebe der Beschluss ohne
// Punkt zurück.
func TestPunktWechseltNichtWaehrendDerAbstimmung(t *testing.T) {
	p := mitTagesordnung(t)
	ctx := context.Background()

	if err := p.kern.TopAufrufen(ctx, 1, 1); err != nil {
		t.Fatalf("punkt 1: %v", err)
	}
	if err := p.kern.AbstimmungStarten(ctx, 1, "Haushalt", kern.AbstimmungOffen); err != nil {
		t.Fatalf("abstimmung: %v", err)
	}

	for name, err := range map[string]error{
		"aufrufen":    p.kern.TopAufrufen(ctx, 1, 3),
		"abschließen": p.kern.TopAbschliessen(ctx, 1),
		"vertagen":    p.kern.TopVertagen(ctx, 1),
	} {
		if code := codeVon(t, err); code != kern.CodeAbstimmungLaeuft {
			t.Errorf("%s: code %q erwartet, %q bekommen", name, kern.CodeAbstimmungLaeuft, code)
		}
	}
	if p.kern.Zustand().Sitzung.AktuellerTop != 1 {
		t.Error("der punkt hat sich trotzdem geändert")
	}

	// Nach dem Auszählen geht es weiter.
	if err := p.kern.AbstimmungBeenden(ctx, 1); err != nil {
		t.Fatalf("auszählen: %v", err)
	}
	if err := p.kern.TopAufrufen(ctx, 1, 3); err != nil {
		t.Errorf("nach dem auszählen muss der wechsel gehen: %v", err)
	}
}

// TestVertagterPunktKommtNichtWieder: er gehört in die nächste Sitzung.
func TestVertagterPunktKommtNichtWieder(t *testing.T) {
	p := mitTagesordnung(t)
	ctx := context.Background()

	if err := p.kern.TopAufrufen(ctx, 1, 2); err != nil {
		t.Fatalf("punkt 2: %v", err)
	}
	if err := p.kern.TopVertagen(ctx, 1); err != nil {
		t.Fatalf("vertagen: %v", err)
	}
	if code := codeVon(t, p.kern.TopAufrufen(ctx, 1, 2)); code != kern.CodeTopGeschlossen {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeTopGeschlossen, code)
	}
	// Ein abgeschlossener Punkt dagegen wird wieder aufgenommen.
	if err := p.kern.TopAufrufen(ctx, 1, 1); err != nil {
		t.Fatalf("punkt 1: %v", err)
	}
	if err := p.kern.TopAbschliessen(ctx, 1); err != nil {
		t.Fatalf("abschließen: %v", err)
	}
	if err := p.kern.TopAufrufen(ctx, 1, 1); err != nil {
		t.Errorf("wiederaufnahme muss gehen: %v", err)
	}
}

// TestNurDieAktiveLeitungFuehrtDurchDieTagesordnung.
func TestNurDieAktiveLeitungFuehrtDurchDieTagesordnung(t *testing.T) {
	p := mitTagesordnung(t)
	ctx := context.Background()

	for _, platz := range []int{2, 4, 5} { // Delegierter, zweite Leitung, Delegierte
		if code := codeVon(t, p.kern.TopAufrufen(ctx, platz, 1)); code != kern.CodeNichtBerechtigt {
			t.Errorf("platz %d: code %q erwartet, %q bekommen", platz, kern.CodeNichtBerechtigt, code)
		}
	}
	if p.kern.Zustand().Sitzung.AktuellerTop != 0 {
		t.Error("es läuft ein punkt, obwohl niemand berechtigt war")
	}
}

// TestPunktUeberlebtDenNeustart: der aufgerufene Punkt und die pausierte
// Aufzeichnung stehen nach einem Absturz wieder richtig.
func TestPunktUeberlebtDenNeustart(t *testing.T) {
	p := mitTagesordnung(t)
	ctx := context.Background()

	if err := p.kern.TopAufrufen(ctx, 1, 2); err != nil {
		t.Fatalf("punkt 2: %v", err)
	}
	nachher := neustarten(t, p)

	z := nachher.Zustand()
	if z.Sitzung.AktuellerTop != 2 {
		t.Errorf("punkt 2 sollte weiter laufen, es läuft %d", z.Sitzung.AktuellerTop)
	}
	if z.Aufzeichnung {
		t.Error("der punkt ist nicht öffentlich — die aufzeichnung muss auch nach dem neustart pausieren")
	}
}

// TestSitzungsendeSchliesstDenPunkt: ein laufender Punkt bliebe sonst für
// immer offen stehen.
func TestSitzungsendeSchliesstDenPunkt(t *testing.T) {
	p := mitTagesordnung(t)
	ctx := context.Background()

	if err := p.kern.TopAufrufen(ctx, 1, 3); err != nil {
		t.Fatalf("punkt 3: %v", err)
	}
	if err := p.kern.SitzungSchliessen(ctx, 1); err != nil {
		t.Fatalf("schließen: %v", err)
	}

	z := p.kern.Zustand()
	if z.Sitzung.AktuellerTop != 0 {
		t.Errorf("nach dem sitzungsende läuft kein punkt mehr, es läuft %d", z.Sitzung.AktuellerTop)
	}
	if z.Aufzeichnung {
		t.Error("nach dem sitzungsende wird nicht aufgezeichnet")
	}
}

// TestOhneTagesordnungBleibtAllesWieVorher: eine Sitzung ohne Tagesordnung
// funktioniert weiter, und der Beschluss braucht dann keinen Punkt.
func TestOhneTagesordnungBleibtAllesWieVorher(t *testing.T) {
	p := aufbauen(t, 5, 3, standardbesetzung())
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2, 4)
	eroeffnen(t, p)

	if err := p.kern.AbstimmungStarten(ctx, 1, "Haushalt", kern.AbstimmungOffen); err != nil {
		t.Fatalf("abstimmung ohne tagesordnung: %v", err)
	}
	if code := codeVon(t, p.kern.TopAufrufen(ctx, 1, 1)); code != kern.CodeAbstimmungLaeuft {
		// Während der Abstimmung greift diese Sperre zuerst — danach muss
		// „keine Tagesordnung" kommen.
		t.Logf("erster code: %q", code)
	}
	if err := p.kern.AbstimmungBeenden(ctx, 1); err != nil {
		t.Fatalf("auszählen: %v", err)
	}
	if code := codeVon(t, p.kern.TopAufrufen(ctx, 1, 1)); code != kern.CodeKeinTop {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeKeinTop, code)
	}
	if !p.kern.Zustand().Aufzeichnung {
		t.Error("ohne tagesordnung wird aufgezeichnet")
	}
}

// TestWiderspruchHaeltDieKameraAn: wer der Aufzeichnung widersprochen hat,
// wird nicht angefahren — sprechen darf er trotzdem.
func TestWiderspruchHaeltDieKameraAn(t *testing.T) {
	besetzung := standardbesetzung()
	besetzung[1].Widerspruch = true // Jonas Öztürk auf Platz 2

	p := aufbauen(t, 5, 3, besetzung)
	ctx := context.Background()
	anmeldenAlle(t, p, 1, 2)
	eroeffnen(t, p)

	if err := p.kern.MikroAn(ctx, 1, 2); err != nil {
		t.Fatalf("mikrofon: %v", err)
	}
	p.kern.KameraAbwarten()

	if abrufe := p.kamera.Abrufe(); len(abrufe) != 0 {
		t.Errorf("die kamera wurde trotz widerspruch gefahren: %+v", abrufe)
	}
	var mikro bool
	for _, pl := range p.kern.Zustand().Plaetze {
		if pl.Nummer == 2 {
			mikro = pl.Mikro
			if !pl.Widerspruch {
				t.Error("der widerspruch steht nicht im zustand")
			}
		}
	}
	if !mikro {
		t.Error("das mikrofon muss trotz widerspruch aufgehen")
	}
	if !hatEreignis(t, p, "kamera_uebersprungen") {
		t.Error("die übersprungene kamerafahrt steht nicht im protokoll")
	}

	// Ein Platz ohne Widerspruch wird weiter angefahren.
	if err := p.kern.MikroAn(ctx, 1, 1); err != nil {
		t.Fatalf("mikrofon platz 1: %v", err)
	}
	p.kern.KameraAbwarten()
	if abrufe := p.kamera.Abrufe(); len(abrufe) != 1 || abrufe[0].Preset != 1 {
		t.Errorf("platz 1 sollte angefahren werden: %+v", abrufe)
	}
}

func hatEreignis(t *testing.T, p *pruefstand, art string) bool {
	t.Helper()
	kette, err := p.ablage.Ereignisse(context.Background(), p.saalID)
	if err != nil {
		t.Fatalf("kette lesen: %v", err)
	}
	for _, e := range kette {
		if e.Art == art {
			return true
		}
	}
	return false
}

func zahlAus(wert any) int {
	switch z := wert.(type) {
	case int:
		return z
	case int64:
		return int(z)
	case float64:
		return int(z)
	default:
		return 0
	}
}
