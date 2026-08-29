// Package ton ist das Regelwerk der Tonstrecke: was aus Akustik und
// Funktechnik folgt, in Code gegossen und getestet. Es rechnet und
// entscheidet — es bewegt selbst keine Samples. Die Signalverarbeitung
// (GStreamer, Anlagentechnik) kommt später dazu und fragt dieses Paket,
// was sie tun darf.
//
// Die Regeln hier sind keine Meinungen. Sie stammen aus der Einmessung
// echter Säle und dürfen nicht wegoptimiert werden.
package ton

import (
	"fmt"
	"math"
	"time"
)

// MaxOffeneMikrofone rechnet die Reserve aus dem Ring-out in die Höchstzahl
// gleichzeitig offener Mikrofone um. Jede Verdopplung offener Mikrofone
// kostet 3 dB Reserve: bei 9 dB sind es acht, bei 3 dB zwei, bei weniger
// als 3 dB bleibt genau eines. Die Zahl folgt aus der Messung, nicht aus
// einer Einstellung — deshalb gibt es hier nichts zu konfigurieren.
func MaxOffeneMikrofone(reserveDB float64) (int, error) {
	if reserveDB < 0 {
		return 0, fmt.Errorf("reserve %.1f dB: unter null heißt, der Saal koppelt schon bei einem Mikrofon — erst einmessen", reserveDB)
	}
	n := int(math.Floor(math.Pow(2, reserveDB/3)))
	if n < 1 {
		n = 1
	}
	return n, nil
}

// Daempfung ist die Absenkung der Summenverstärkung bei n offenen
// Mikrofonen, in dB (positiv). Das ist das NOM-Gesetz des Automixers:
// 10·log10(n) — die Verdopplung kostet 3 dB, damit die Gesamtverstärkung
// im Saal gleich bleibt, egal wie viele Mikrofone offen sind.
func Daempfung(offene int) float64 {
	if offene <= 1 {
		return 0
	}
	return 10 * math.Log10(float64(offene))
}

// Latenzbudgets. Wer sie reißt, hört es: unter 10 ms vom Mikrofon zur
// Saalbeschallung, sonst entstehen Kammfiltereffekte mit der echten Stimme
// im Raum. Unter 60 ms zum Dolmetschkopfhörer, weil der Zuhörer den Saal
// parallel hört.
const (
	BudgetSaal      = 10 * time.Millisecond
	BudgetDolmetsch = 60 * time.Millisecond
)

// Weg ist eine Tonstrecke mit Budget.
type Weg string

const (
	WegSaal      Weg = "mikrofon→saal"
	WegDolmetsch Weg = "dolmetsch→kopfhörer"
)

// BudgetGeprueft sagt, ob eine gemessene Latenz auf diesem Weg tragbar ist.
// Der Fehlertext erklärt die Folge, nicht nur die Zahl.
func BudgetGeprueft(weg Weg, latenz time.Duration) error {
	switch weg {
	case WegSaal:
		if latenz > BudgetSaal {
			return fmt.Errorf("%s: %v über dem Budget von %v — Kammfiltereffekte mit der echten Stimme im Raum",
				weg, latenz, BudgetSaal)
		}
	case WegDolmetsch:
		if latenz > BudgetDolmetsch {
			return fmt.Errorf("%s: %v über dem Budget von %v — der Zuhörer hört den Saal parallel und verliert den Anschluss",
				weg, latenz, BudgetDolmetsch)
		}
	default:
		return fmt.Errorf("unbekannter weg %q", weg)
	}
	return nil
}

// HochpassGueltig prüft die Grenzfrequenz des Hochpasses je Kanal.
// 100 bis 120 Hz gegen Tischrumpeln — tiefer lässt das Rumpeln durch,
// höher dünnt Stimmen aus.
func HochpassGueltig(hz float64) error {
	if hz < 100 || hz > 120 {
		return fmt.Errorf("hochpass bei %.0f Hz: gegen Tischrumpeln gehören 100 bis 120 Hz", hz)
	}
	return nil
}
