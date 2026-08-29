package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func schreibeKonfiguration(t *testing.T, inhalt string) string {
	t.Helper()
	pfad := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(pfad, []byte(inhalt), 0o600); err != nil {
		t.Fatalf("konfiguration schreiben: %v", err)
	}
	return pfad
}

// TestEinmessungTraegtDieHoechstzahl: ist die Reserve hinterlegt, folgt die
// Höchstzahl offener Mikrofone aus ihr — und eine Konfiguration, die mehr
// verlangt, wird beim Start abgewiesen, nicht leiser gestellt.
func TestEinmessungTraegtDieHoechstzahl(t *testing.T) {
	// Reserve gesetzt, Höchstzahl offen gelassen: 9 dB tragen 8.
	k, err := KonfigurationLesen(schreibeKonfiguration(t, `
datenbank: gedaechtnis
einmessung_reserve_db: 9
`))
	if err != nil {
		t.Fatalf("lesen: %v", err)
	}
	if k.MaxOffeneMikrofone != 8 {
		t.Errorf("9 dB reserve tragen 8 mikrofone, konfiguriert wurden %d", k.MaxOffeneMikrofone)
	}

	// Konfiguration verlangt mehr, als die Messung trägt: Ablehnung mit Ansage.
	_, err = KonfigurationLesen(schreibeKonfiguration(t, `
datenbank: gedaechtnis
max_offene_mikrofone: 8
einmessung_reserve_db: 3
`))
	if err == nil || !strings.Contains(err.Error(), "3 dB") {
		t.Fatalf("die überzogene konfiguration muss mit begründung scheitern, kam: %v", err)
	}

	// Weniger als die Messung erlaubt ist in Ordnung — vorsichtig darf man sein.
	k, err = KonfigurationLesen(schreibeKonfiguration(t, `
datenbank: gedaechtnis
max_offene_mikrofone: 4
einmessung_reserve_db: 9
`))
	if err != nil || k.MaxOffeneMikrofone != 4 {
		t.Fatalf("eine vorsichtige konfiguration muss stehen bleiben: %v, max %d", err, k.MaxOffeneMikrofone)
	}

	// Ganz ohne beides: die alte Pflichtangabe greift weiter.
	if _, err := KonfigurationLesen(schreibeKonfiguration(t, "datenbank: gedaechtnis\n")); err == nil {
		t.Fatal("ohne höchstzahl und ohne messung muss das lesen scheitern")
	}
}
