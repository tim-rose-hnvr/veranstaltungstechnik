package web

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kamera"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
)

// Emulatordaten ist alles, was die Prüfstelle zusätzlich zum laufenden System
// braucht — vor allem die PINs im Klartext.
//
// Deshalb wird sie nur eingerichtet, wenn es in der Konfiguration ausdrücklich
// steht. Ohne diesen Schalter gibt es die Adressen unter /emulator nicht: sie
// sind dann nicht abgeschaltet, sondern nicht vorhanden.
type Emulatordaten struct {
	Saal     string
	SaalID   string
	Plaetze  []kern.Platzaufbau
	Pins     map[int]string
	Kameras  []Kameraangabe
	Attrappe *kamera.Attrappe // nil, wenn echte Kameras im Netz hängen
	Kette    Kettenleser
}

// Kameraangabe beschreibt eine Kamera für die Prüfstelle.
type Kameraangabe struct {
	Name    string `json:"name"`
	Adresse string `json:"adresse"`
	Kanal   uint8  `json:"kanal"`
}

// Kettenleser liefert die Ereigniskette eines Saals.
type Kettenleser interface {
	Ereignisse(ctx context.Context, saalID string) ([]kern.Ereignis, error)
}

// SetzeEmulator schaltet die Prüfstelle frei.
func (s *Server) SetzeEmulator(d Emulatordaten) { s.emulator = &d }

// emulatorRouten hängt die Adressen der Prüfstelle ein. Ohne freigeschalteten
// Emulator wird sie nicht aufgerufen.
func (s *Server) emulatorRouten(weiche *http.ServeMux) {
	weiche.Handle("GET /emulator", s.seite("emulator.html"))
	weiche.HandleFunc("GET /emulator/aufbau.json", s.emulatorAufbau)
	weiche.HandleFunc("GET /emulator/kette.json", s.emulatorKette)
	weiche.HandleFunc("GET /emulator/kameras.json", s.emulatorKameras)
}

// emulatorPlatz ist ein Platz samt PIN und Kameravorgabe.
type emulatorPlatz struct {
	Nummer      int    `json:"nummer"`
	Name        string `json:"name"`
	Person      string `json:"person"`
	Rolle       string `json:"rolle"`
	Pin         string `json:"pin"`
	Kamera      string `json:"kamera"`
	Preset      uint8  `json:"preset"`
	Widerspruch bool   `json:"widerspruch"`
}

func (s *Server) emulatorAufbau(w http.ResponseWriter, r *http.Request) {
	z := s.kern.Zustand()

	plaetze := make([]emulatorPlatz, 0, len(s.emulator.Plaetze))
	for _, a := range s.emulator.Plaetze {
		e := emulatorPlatz{
			Nummer: a.Nummer, Name: a.Name, Kamera: a.KameraName, Preset: a.Preset,
			Pin: s.emulator.Pins[a.Nummer],
		}
		for _, p := range z.Plaetze {
			if p.Nummer == a.Nummer {
				e.Person = p.Person
				e.Widerspruch = p.Widerspruch
			}
		}
		if ich := s.kern.Ich(a.Nummer); ich != nil {
			e.Rolle = string(ich.Rolle)
		}
		plaetze = append(plaetze, e)
	}

	s.alsJSON(w, map[string]any{
		"saal":      s.emulator.Saal,
		"max_offen": z.MaxOffen,
		"plaetze":   plaetze,
		"kameras":   s.emulator.Kameras,
		"attrappe":  s.emulator.Attrappe != nil,
	})
}

// emulatorKetteEintrag ist ein Kettenglied, wie es die Prüfstelle zeigt.
type emulatorKetteEintrag struct {
	FolgeNr  int64          `json:"folge_nr"`
	Zeit     string         `json:"zeit"`
	Art      string         `json:"art"`
	Nutzlast map[string]any `json:"nutzlast"`
	Hash     string         `json:"hash"`
}

func (s *Server) emulatorKette(w http.ResponseWriter, r *http.Request) {
	if s.emulator.Kette == nil {
		s.alsJSON(w, map[string]any{"eintraege": []any{}, "geprueft": false,
			"fehler": "Die Kette ist nicht eingerichtet."})
		return
	}

	kette, err := s.emulator.Kette.Ereignisse(r.Context(), s.emulator.SaalID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.alsJSON(w, map[string]any{"fehler": err.Error()})
		return
	}

	eintraege := make([]emulatorKetteEintrag, 0, len(kette))
	for _, e := range kette {
		eintraege = append(eintraege, emulatorKetteEintrag{
			FolgeNr: e.FolgeNr, Zeit: kern.ZeitText(e.Zeit), Art: e.Art,
			Nutzlast: e.Nutzlast, Hash: hex.EncodeToString(e.Hash),
		})
	}

	antwort := map[string]any{"eintraege": eintraege, "geprueft": true}
	if err := kern.KettePruefen(kette); err != nil {
		antwort["geprueft"] = false
		antwort["fehler"] = err.Error()
	}
	s.alsJSON(w, antwort)
}

func (s *Server) emulatorKameras(w http.ResponseWriter, r *http.Request) {
	if s.emulator.Attrappe == nil {
		s.alsJSON(w, map[string]any{
			"attrappe": false,
			"hinweis":  "Es sind echte Kameras eingetragen — was auf dem Netz passiert, sieht nur die Kamera.",
		})
		return
	}
	s.alsJSON(w, map[string]any{
		"attrappe": true,
		"staende":  s.emulator.Attrappe.Staende(),
		"empfang":  s.emulator.Attrappe.Empfangen(),
	})
}

func (s *Server) alsJSON(w http.ResponseWriter, inhalt any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(inhalt); err != nil {
		s.protokoll.Error("antwort nicht verpackbar", "grund", err)
	}
}
