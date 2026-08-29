package ton_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/ton"
)

// TestVerdopplungKostetDreiDezibel: die Höchstzahl offener Mikrofone folgt
// aus der Reserve des Ring-outs — jede Verdopplung kostet 3 dB. Typisch
// sind 9 dB Reserve und damit acht Mikrofone.
func TestVerdopplungKostetDreiDezibel(t *testing.T) {
	faelle := []struct {
		reserve float64
		will    int
	}{
		{0, 1},   // keine Reserve: genau ein Mikrofon
		{2.9, 1}, // knapp unter der ersten Verdopplung
		{3, 2},   // eine Verdopplung
		{6, 4},   // zwei
		{9, 8},   // drei — der typische Saal
		{10.5, 11},
	}
	for _, f := range faelle {
		n, err := ton.MaxOffeneMikrofone(f.reserve)
		if err != nil {
			t.Fatalf("reserve %.1f: %v", f.reserve, err)
		}
		if n != f.will {
			t.Errorf("reserve %.1f dB: %d mikrofone erwartet, %d bekommen", f.reserve, f.will, n)
		}
	}

	// Unter null ist keine Rechnung, sondern ein Befund.
	if _, err := ton.MaxOffeneMikrofone(-1); err == nil ||
		!strings.Contains(err.Error(), "einmessen") {
		t.Errorf("negative reserve muss zum einmessen schicken, kam: %v", err)
	}
}

// TestAutomixerHaeltDieSumme: das NOM-Gesetz. Bei doppelt so vielen offenen
// Mikrofonen sinkt die Summenverstärkung um 3 dB, damit die
// Gesamtverstärkung im Saal gleich bleibt.
func TestAutomixerHaeltDieSumme(t *testing.T) {
	if d := ton.Daempfung(1); d != 0 {
		t.Errorf("ein mikrofon braucht keine dämpfung, %.2f bekommen", d)
	}
	fuerZwei := ton.Daempfung(2)
	if fuerZwei < 2.9 || fuerZwei > 3.1 {
		t.Errorf("zwei mikrofone: rund 3 dB erwartet, %.2f bekommen", fuerZwei)
	}
	// Verdopplung von 4 auf 8: wieder rund 3 dB obendrauf.
	sprung := ton.Daempfung(8) - ton.Daempfung(4)
	if sprung < 2.9 || sprung > 3.1 {
		t.Errorf("die verdopplung 4→8 muss rund 3 dB kosten, kostet %.2f", sprung)
	}
}

// TestLatenzbudgets: die Grenzen sind hörphysikalisch, nicht verhandelbar.
func TestLatenzbudgets(t *testing.T) {
	if err := ton.BudgetGeprueft(ton.WegSaal, 9*time.Millisecond); err != nil {
		t.Errorf("9 ms zum saal sind im budget: %v", err)
	}
	err := ton.BudgetGeprueft(ton.WegSaal, 12*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "Kammfilter") {
		t.Errorf("12 ms zum saal müssen am kammfilter scheitern, kam: %v", err)
	}
	if err := ton.BudgetGeprueft(ton.WegDolmetsch, 55*time.Millisecond); err != nil {
		t.Errorf("55 ms zum dolmetschkopfhörer sind im budget: %v", err)
	}
	if err := ton.BudgetGeprueft(ton.WegDolmetsch, 80*time.Millisecond); err == nil {
		t.Error("80 ms zum dolmetschkopfhörer müssen abgewiesen werden")
	}
	if err := ton.BudgetGeprueft("irgendwas", time.Millisecond); err == nil {
		t.Error("ein unbekannter weg darf nicht durchgewunken werden")
	}
}

// TestHochpassGegenTischrumpeln: 100 bis 120 Hz, sonst Fehler.
func TestHochpassGegenTischrumpeln(t *testing.T) {
	for _, gut := range []float64{100, 110, 120} {
		if err := ton.HochpassGueltig(gut); err != nil {
			t.Errorf("%.0f Hz sind gültig: %v", gut, err)
		}
	}
	for _, schlecht := range []float64{80, 99, 121, 200} {
		if err := ton.HochpassGueltig(schlecht); err == nil {
			t.Errorf("%.0f Hz müssen abgewiesen werden", schlecht)
		}
	}
}

// TestMixMinusLaesstDieEigeneQuelleWeg: keine Summe enthält ihre eigene
// Quelle — sonst entsteht zwischen Saal und Zuschaltung die Schleife.
func TestMixMinusLaesstDieEigeneQuelleWeg(t *testing.T) {
	quellen := []ton.Quelle{"saal", "zuschaltung", "dolmetsch_en"}
	matrix, err := ton.MixMinus(quellen)
	if err != nil {
		t.Fatalf("matrix: %v", err)
	}
	if len(matrix) != 3 {
		t.Fatalf("drei ausgänge erwartet, %d bekommen", len(matrix))
	}
	for ausgang, summe := range matrix {
		if len(summe) != 2 {
			t.Errorf("ausgang %q: zwei zuspieler erwartet, %d bekommen", ausgang, len(summe))
		}
		for _, q := range summe {
			if q == ausgang {
				t.Errorf("ausgang %q hört sich selbst — genau das ist die schleife", ausgang)
			}
		}
	}

	if _, err := ton.MixMinus([]ton.Quelle{"saal", "saal"}); err == nil {
		t.Error("eine doppelte quelle muss abgewiesen werden")
	}
	if _, err := ton.MixMinus([]ton.Quelle{"saal", ""}); err == nil {
		t.Error("eine namenlose quelle muss abgewiesen werden")
	}
}

// TestFilterhaushalt: höchstens 6 bis 8 schmale Filter, feste weichen nie,
// kurzlebige laufen aus und werden bei Not verdrängt — der älteste zuerst.
func TestFilterhaushalt(t *testing.T) {
	if _, err := ton.NeuHaushalt(9); err == nil {
		t.Fatal("neun filter müssen abgewiesen werden — der klang stirbt")
	}
	if _, err := ton.NeuHaushalt(5); err == nil {
		t.Fatal("fünf sind zu wenige für einen schwierigen saal")
	}
	h, err := ton.NeuHaushalt(6)
	if err != nil {
		t.Fatalf("sechs sind erlaubt: %v", err)
	}

	jetzt := time.Now()
	// Vier feste aus dem Ring-out, zwei kurzlebige — voll.
	for _, f := range []float64{215, 480, 1250, 3150} {
		if err := h.FestSetzen(f); err != nil {
			t.Fatalf("fester filter %.0f: %v", f, err)
		}
	}
	if err := h.Daempfen(920, jetzt); err != nil {
		t.Fatalf("kurzlebig 920: %v", err)
	}
	if err := h.Daempfen(2400, jetzt.Add(time.Second)); err != nil {
		t.Fatalf("kurzlebig 2400: %v", err)
	}

	// Der siebte verdrängt den ältesten kurzlebigen (920), nie einen festen.
	if err := h.Daempfen(5100, jetzt.Add(2*time.Second)); err != nil {
		t.Fatalf("kurzlebig 5100: %v", err)
	}
	frequenzen := map[float64]bool{}
	for _, f := range h.Stehende() {
		frequenzen[f.Frequenz] = true
	}
	if frequenzen[920] {
		t.Error("der älteste kurzlebige musste weichen")
	}
	for _, fest := range []float64{215, 480, 1250, 3150} {
		if !frequenzen[fest] {
			t.Errorf("der feste filter %.0f darf nie verdrängt werden", fest)
		}
	}

	// Nach Ablauf klingen kurzlebige ab, feste bleiben.
	h.Abklingen(jetzt.Add(10 * time.Minute))
	if rest := len(h.Stehende()); rest != 4 {
		t.Errorf("nach dem abklingen sollten 4 feste stehen, es stehen %d", rest)
	}

	// Sind alle Plätze fest belegt, hilft kein weiterer Filter — Fehler mit Rat.
	voll, _ := ton.NeuHaushalt(6)
	for _, f := range []float64{100, 200, 300, 400, 500, 600} {
		if err := voll.FestSetzen(f); err != nil {
			t.Fatalf("fest %.0f: %v", f, err)
		}
	}
	if err := voll.FestSetzen(700); err == nil || !strings.Contains(err.Error(), "einmessung") {
		t.Errorf("der siebte feste braucht eine ansage zur einmessung, kam: %v", err)
	}
	if err := voll.Daempfen(800, jetzt); err == nil || !strings.Contains(err.Error(), "verstärkung senken") {
		t.Errorf("koppelt es trotz vollem haushalt, ist der rat 'verstärkung senken', kam: %v", err)
	}
}
