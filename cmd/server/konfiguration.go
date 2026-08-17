package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Konfiguration ist der Inhalt von config.yaml.
type Konfiguration struct {
	Datenbank          string `yaml:"datenbank"`
	Adresse            string `yaml:"adresse"`
	SaalDatei          string `yaml:"saal_datei"`
	SitzungsDatei      string `yaml:"sitzungs_datei"`
	MaxOffeneMikrofone int    `yaml:"max_offene_mikrofone"`
	KameraZeitlimitMs  int    `yaml:"kamera_zeitlimit_ms"`
}

// KonfigurationLesen liest config.yaml und prüft die Pflichtangaben.
func KonfigurationLesen(pfad string) (Konfiguration, error) {
	roh, err := os.ReadFile(pfad)
	if err != nil {
		return Konfiguration{}, fmt.Errorf("konfiguration %s nicht lesbar: %w", pfad, err)
	}

	var k Konfiguration
	if err := yaml.Unmarshal(roh, &k); err != nil {
		return Konfiguration{}, fmt.Errorf("konfiguration %s nicht lesbar: %w", pfad, err)
	}

	if k.Datenbank == "" {
		return Konfiguration{}, fmt.Errorf("konfiguration %s: feld \"datenbank\" fehlt", pfad)
	}
	if k.Adresse == "" {
		k.Adresse = ":8080"
	}
	if k.SaalDatei == "" {
		k.SaalDatei = "saal.json"
	}
	if k.SitzungsDatei == "" {
		k.SitzungsDatei = "sitzung.json"
	}
	if k.MaxOffeneMikrofone < 1 {
		return Konfiguration{}, fmt.Errorf("konfiguration %s: max_offene_mikrofone muss mindestens 1 sein", pfad)
	}
	if k.KameraZeitlimitMs < 1 {
		k.KameraZeitlimitMs = 500
	}
	return k, nil
}

// KameraZeitlimit ist das Zeitlimit als Dauer.
func (k Konfiguration) KameraZeitlimit() time.Duration {
	return time.Duration(k.KameraZeitlimitMs) * time.Millisecond
}
