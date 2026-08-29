package ton

import "fmt"

// Quelle ist ein Zuspieler in der Tonmatrix: der Saal selbst, eine
// Zuschaltung von außen, ein Dolmetschkanal, die Aufzeichnung.
type Quelle string

// MixMinus baut je Ausgang die Summe aller Quellen ohne die eigene.
// Ohne diese Auslassung entsteht zwischen Saal und Zuschaltung eine
// Schleife: die Zuschaltung hört sich selbst mit Verzögerung wieder und
// schaukelt sich auf. Die Aufzeichnung ist kein Zuspieler und bekommt
// deshalb alles — sie kommt als Ausgang dazu, nie als Quelle.
func MixMinus(quellen []Quelle) (map[Quelle][]Quelle, error) {
	gesehen := map[Quelle]bool{}
	for _, q := range quellen {
		if q == "" {
			return nil, fmt.Errorf("eine quelle ohne namen kann niemand aus seiner summe lassen")
		}
		if gesehen[q] {
			return nil, fmt.Errorf("quelle %q doppelt: die zweite bekäme die erste zu hören — schleife", q)
		}
		gesehen[q] = true
	}

	matrix := make(map[Quelle][]Quelle, len(quellen))
	for _, ausgang := range quellen {
		summe := make([]Quelle, 0, len(quellen)-1)
		for _, q := range quellen {
			if q != ausgang {
				summe = append(summe, q)
			}
		}
		matrix[ausgang] = summe
	}
	return matrix, nil
}
