package ton

import (
	"fmt"
	"sort"
	"time"
)

// Zwei Sorten Rückkopplungsfilter: feste aus dem Ring-out, die bleiben,
// und kurzlebige für neu auftretende Frequenzen, die wieder auslaufen.
// Zusammen höchstens sechs bis acht schmale Filter — mehr, und der Klang
// stirbt: jeder schmale Filter reißt ein Loch ins Spektrum.

// Filterart unterscheidet die beiden Sorten.
type Filterart string

const (
	FilterFest      Filterart = "fest"      // aus dem Ring-out, bleibt
	FilterKurzlebig Filterart = "kurzlebig" // klingt nach Ablauf wieder ab
)

// Filter ist ein schmalbandiger Absenker auf einer Koppelfrequenz.
type Filter struct {
	Frequenz float64 // Hz
	Art      Filterart
	Bis      time.Time // nur kurzlebig: wann er ausläuft
}

// KurzlebigDauer: wie lange ein kurzlebiger Filter stehen bleibt, bevor er
// abklingt. Lange genug, dass die Koppelneigung vorbei ist; kurz genug,
// dass ein einmaliger Ausrutscher den Klang nicht dauerhaft verstellt.
const KurzlebigDauer = 90 * time.Second

// Haushalt verwaltet die Filter unter der Obergrenze.
type Haushalt struct {
	grenze int
	filter []Filter
}

// NeuHaushalt legt den Filterhaushalt an. Erlaubt sind sechs bis acht —
// die Spanne aus der Praxis, mehr stirbt der Klang, weniger reicht in
// schwierigen Sälen nicht.
func NeuHaushalt(grenze int) (*Haushalt, error) {
	if grenze < 6 || grenze > 8 {
		return nil, fmt.Errorf("%d schmale filter: erlaubt sind 6 bis 8 — mehr reißt hörbare löcher ins spektrum", grenze)
	}
	return &Haushalt{grenze: grenze}, nil
}

// FestSetzen trägt einen Filter aus dem Ring-out ein. Feste Filter werden
// nie verdrängt; passt keiner mehr, ist der Saal nicht beherrschbar und
// die Antwort ein Fehler, kein stilles Weglassen.
func (h *Haushalt) FestSetzen(frequenz float64) error {
	if frequenz <= 0 {
		return fmt.Errorf("frequenz %.0f Hz gibt es nicht", frequenz)
	}
	feste := 0
	for _, f := range h.filter {
		if f.Art == FilterFest {
			feste++
		}
	}
	if feste >= h.grenze {
		return fmt.Errorf("alle %d filter sind fest belegt — der saal braucht eine neue einmessung oder weniger verstärkung, nicht noch einen filter", h.grenze)
	}
	// Ein fester Filter verdrängt notfalls den ältesten kurzlebigen.
	if len(h.filter) >= h.grenze {
		h.aeltestenKurzlebigenEntfernen()
	}
	h.filter = append(h.filter, Filter{Frequenz: frequenz, Art: FilterFest})
	return nil
}

// Daempfen setzt einen kurzlebigen Filter auf eine neu aufgetretene
// Koppelfrequenz. Ist der Haushalt voll, weicht der älteste kurzlebige —
// feste weichen nie.
func (h *Haushalt) Daempfen(frequenz float64, jetzt time.Time) error {
	if frequenz <= 0 {
		return fmt.Errorf("frequenz %.0f Hz gibt es nicht", frequenz)
	}
	h.Abklingen(jetzt)
	if len(h.filter) >= h.grenze && !h.aeltestenKurzlebigenEntfernen() {
		return fmt.Errorf("alle %d filter sind fest belegt — es koppelt trotz ring-out: verstärkung senken", h.grenze)
	}
	h.filter = append(h.filter, Filter{Frequenz: frequenz, Art: FilterKurzlebig, Bis: jetzt.Add(KurzlebigDauer)})
	return nil
}

// Abklingen räumt abgelaufene kurzlebige Filter aus.
func (h *Haushalt) Abklingen(jetzt time.Time) {
	behalten := h.filter[:0]
	for _, f := range h.filter {
		if f.Art == FilterKurzlebig && !f.Bis.After(jetzt) {
			continue
		}
		behalten = append(behalten, f)
	}
	h.filter = behalten
}

// Stehende liefert die Filter, nach Frequenz geordnet.
func (h *Haushalt) Stehende() []Filter {
	aus := append([]Filter(nil), h.filter...)
	sort.Slice(aus, func(i, j int) bool { return aus[i].Frequenz < aus[j].Frequenz })
	return aus
}

func (h *Haushalt) aeltestenKurzlebigenEntfernen() bool {
	aeltester := -1
	for i, f := range h.filter {
		if f.Art != FilterKurzlebig {
			continue
		}
		if aeltester == -1 || f.Bis.Before(h.filter[aeltester].Bis) {
			aeltester = i
		}
	}
	if aeltester == -1 {
		return false
	}
	h.filter = append(h.filter[:aeltester], h.filter[aeltester+1:]...)
	return true
}
