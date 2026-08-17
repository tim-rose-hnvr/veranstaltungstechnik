package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/speicher"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/vorabcheck"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/web"
)

// Diese Tests fahren das WebSocket-Protokoll über eine echte HTTP-Verbindung.
// Sie halten fest, was auf der Leitung steht — nicht nur, was der Kern denkt.

type stilleKamera struct{}

func (stilleKamera) PresetAbrufen(ctx context.Context, adresse string, kanal, preset uint8) error {
	return nil
}

// nachricht ist die Sicht des Clients auf den Zustand.
type nachricht struct {
	Typ     string `json:"typ"`
	Code    string `json:"code"`
	Text    string `json:"text"`
	Stand   uint64 `json:"stand"`
	Sitzung struct {
		Titel        string `json:"titel"`
		Zustand      string `json:"zustand"`
		LeitungPlatz int    `json:"leitung_platz"`
	} `json:"sitzung"`
	Ich *struct {
		Platz  int      `json:"platz"`
		Person string   `json:"person"`
		Rolle  string   `json:"rolle"`
		Darf   []string `json:"darf"`
	} `json:"ich"`
	Plaetze []struct {
		Nummer  int    `json:"nummer"`
		Person  string `json:"person"`
		Mikro   bool   `json:"mikro"`
		Belegt  bool   `json:"belegt"`
		HatWort bool   `json:"hat_wort"`
	} `json:"plaetze"`
	Redeliste []struct {
		Platz   int    `json:"platz"`
		Person  string `json:"person"`
		Zustand string `json:"zustand"`
	} `json:"redeliste"`
}

func aufstellen(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	ablage := speicher.NeuGedaechtnis()

	saalID, plaetze, err := ablage.SaalImportieren(ctx, speicher.Saaldaten{
		Saal:    "Testraum",
		Kameras: []speicher.Kameradaten{{Name: "PTZ Mitte", Adresse: "127.0.0.1:52381", Kanal: 1}},
		Plaetze: []speicher.Platzdaten{
			{Nummer: 1, Name: "Vorsitz", Kamera: "PTZ Mitte", Preset: 1},
			{Nummer: 2, Name: "Platz 2", Kamera: "PTZ Mitte", Preset: 2},
			{Nummer: 3, Name: "Platz 3", Kamera: "PTZ Mitte", Preset: 3},
		},
	})
	if err != nil {
		t.Fatalf("saal einlesen: %v", err)
	}

	stand, err := ablage.SitzungImportieren(ctx, saalID, speicher.Sitzungsdaten{
		Titel: "Probesitzung",
		Teilnahmen: []speicher.Teilnahmedaten{
			{Platz: 1, Person: "Anke Bergmann", Rolle: "leitung", Pin: "1111"},
			{Platz: 2, Person: "Jonas Öztürk", Rolle: "delegierter", Pin: "2222"},
			{Platz: 3, Person: "Rita Falk", Rolle: "schriftfuehrung", Pin: "3333"},
		},
	})
	if err != nil {
		t.Fatalf("sitzung einlesen: %v", err)
	}

	aufbau := kern.Aufbau{
		SaalID: saalID, SitzungID: stand.SitzungID, Titel: stand.Titel,
		SitzungZustand: stand.Zustand, Plaetze: plaetze, Teilnahmen: stand.Teilnahmen,
		MaxOffen: 3, Zeitlimit: 50 * time.Millisecond,
	}
	k, err := kern.Neu(aufbau, stilleKamera{}, ablage, nil)
	if err != nil {
		t.Fatalf("kern nicht aufgebaut: %v", err)
	}

	oberflaeche := web.Neu(k, "../../web", nil)
	oberflaeche.SetzeVorabcheck(vorabcheck.Neu(aufbau, k, stilleKamera{}, ablage, 50*time.Millisecond))

	dienst := httptest.NewServer(oberflaeche.Handler())
	t.Cleanup(dienst.Close)
	return dienst
}

type client struct {
	t    *testing.T
	sitz *websocket.Conn
	ctx  context.Context
}

func verbinden(t *testing.T, dienst *httptest.Server) *client {
	t.Helper()
	ctx, ende := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(ende)

	adresse := "ws" + strings.TrimPrefix(dienst.URL, "http") + "/ws"
	sitz, _, err := websocket.Dial(ctx, adresse, nil)
	if err != nil {
		t.Fatalf("verbinden: %v", err)
	}
	t.Cleanup(func() { sitz.CloseNow() }) //nolint:errcheck

	k := &client{t: t, sitz: sitz, ctx: ctx}
	k.warte(func(n nachricht) bool { return n.Typ == "zustand" }) // Anfangszustand
	return k
}

func (k *client) schicken(inhalt map[string]any) {
	k.t.Helper()
	roh, err := json.Marshal(inhalt)
	if err != nil {
		k.t.Fatalf("nachricht verpacken: %v", err)
	}
	if err := k.sitz.Write(k.ctx, websocket.MessageText, roh); err != nil {
		k.t.Fatalf("senden: %v", err)
	}
}

// warte liest, bis die Bedingung zutrifft oder ein Fehler kommt.
func (k *client) warte(passt func(nachricht) bool) nachricht {
	k.t.Helper()
	frist, ende := context.WithTimeout(k.ctx, 3*time.Second)
	defer ende()

	for {
		_, roh, err := k.sitz.Read(frist)
		if err != nil {
			k.t.Fatalf("lesen: %v", err)
		}
		var n nachricht
		if err := json.Unmarshal(roh, &n); err != nil {
			k.t.Fatalf("nachricht nicht lesbar: %v", err)
		}
		if n.Typ == "fehler" || passt(n) {
			return n
		}
	}
}

func (k *client) anmelden(platz int, pin string) nachricht {
	k.t.Helper()
	k.schicken(map[string]any{"typ": "anmelden", "platz": platz, "pin": pin})
	return k.warte(func(n nachricht) bool { return n.Ich != nil })
}

func enthaelt(liste []string, wert string) bool {
	for _, e := range liste {
		if e == wert {
			return true
		}
	}
	return false
}

// TestProtokollAnmeldung: falsche PIN wird abgewiesen, richtige liefert den
// eigenen Zustand — und der ist je Verbindung verschieden.
func TestProtokollAnmeldung(t *testing.T) {
	dienst := aufstellen(t)
	anke := verbinden(t, dienst)
	jonas := verbinden(t, dienst)

	anke.schicken(map[string]any{"typ": "anmelden", "platz": 1, "pin": "0000"})
	if n := anke.warte(func(n nachricht) bool { return n.Typ == "fehler" }); n.Code != kern.CodePinFalsch {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodePinFalsch, n.Code)
	}

	n := anke.anmelden(1, "1111")
	if n.Ich.Person != "Anke Bergmann" || n.Ich.Rolle != string(kern.RolleLeitung) {
		t.Fatalf("falscher eigener zustand: %+v", n.Ich)
	}
	if !enthaelt(n.Ich.Darf, kern.AktionSitzungEroeffnen) {
		t.Errorf("die leitung sollte eröffnen dürfen, darf: %v", n.Ich.Darf)
	}

	// Dieselbe Änderung, andere Verbindung: "ich" ist die einzige Stelle,
	// an der nicht alle dasselbe bekommen.
	zweite := jonas.warte(func(n nachricht) bool { return n.Plaetze[0].Belegt })
	if zweite.Ich != nil {
		t.Errorf("ein nicht angemeldeter client darf kein ich bekommen: %+v", zweite.Ich)
	}
}

// TestProtokollWortUndMikro: der ganze Weg über die Leitung — melden,
// erteilen, sprechen.
func TestProtokollWortUndMikro(t *testing.T) {
	dienst := aufstellen(t)
	anke := verbinden(t, dienst)
	jonas := verbinden(t, dienst)

	anke.anmelden(1, "1111")
	jonas.anmelden(2, "2222")

	anke.schicken(map[string]any{"typ": kern.AktionSitzungEroeffnen})
	anke.warte(func(n nachricht) bool { return n.Sitzung.Zustand == string(kern.SitzungLaufend) })

	// Ohne erteiltes Wort bleibt das Mikrofon zu.
	jonas.schicken(map[string]any{"typ": "mikro_an", "platz": 2})
	if n := jonas.warte(func(n nachricht) bool { return n.Typ == "fehler" }); n.Code != kern.CodeKeinWort {
		t.Fatalf("code %q erwartet, %q bekommen", kern.CodeKeinWort, n.Code)
	}

	jonas.schicken(map[string]any{"typ": kern.AktionWortMelden})
	n := jonas.warte(func(n nachricht) bool { return len(n.Redeliste) == 1 })
	if n.Redeliste[0].Person != "Jonas Öztürk" || n.Redeliste[0].Zustand != string(kern.WortGemeldet) {
		t.Fatalf("redeliste falsch: %+v", n.Redeliste[0])
	}

	anke.schicken(map[string]any{"typ": kern.AktionWortErteilen, "platz": 2})
	n = jonas.warte(func(n nachricht) bool { return n.Plaetze[1].HatWort })
	if n.Redeliste[0].Zustand != string(kern.WortErteilt) {
		t.Errorf("erteilt erwartet, %q bekommen", n.Redeliste[0].Zustand)
	}

	jonas.schicken(map[string]any{"typ": "mikro_an", "platz": 2})
	n = jonas.warte(func(n nachricht) bool { return n.Plaetze[1].Mikro })
	if n.Redeliste[0].Zustand != string(kern.WortLaufend) {
		t.Errorf("laufend erwartet, %q bekommen", n.Redeliste[0].Zustand)
	}

	// Der Entzug schließt das Mikrofon und räumt die Liste.
	anke.schicken(map[string]any{"typ": kern.AktionWortEntziehen, "platz": 2})
	n = anke.warte(func(n nachricht) bool { return len(n.Redeliste) == 0 })
	if n.Plaetze[1].Mikro {
		t.Error("das mikrofon sollte nach dem entzug zu sein")
	}
}

// TestProtokollRechteImKern: die Oberfläche filtert nichts weg. Ein Befehl
// ohne Recht geht durch bis zum Kern und wird dort abgewiesen.
func TestProtokollRechteImKern(t *testing.T) {
	dienst := aufstellen(t)
	anke := verbinden(t, dienst)
	rita := verbinden(t, dienst)
	fremder := verbinden(t, dienst)

	anke.anmelden(1, "1111")
	rita.anmelden(3, "3333")
	anke.schicken(map[string]any{"typ": kern.AktionSitzungEroeffnen})
	anke.warte(func(n nachricht) bool { return n.Sitzung.Zustand == string(kern.SitzungLaufend) })

	// Schriftführung darf nicht schalten.
	rita.schicken(map[string]any{"typ": "mikro_an", "platz": 3})
	if n := rita.warte(func(n nachricht) bool { return n.Typ == "fehler" }); n.Code != kern.CodeNichtBerechtigt {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeNichtBerechtigt, n.Code)
	}

	// Wer gar nicht angemeldet ist, darf erst recht nichts.
	fremder.schicken(map[string]any{"typ": kern.AktionWortErteilen, "platz": 2})
	if n := fremder.warte(func(n nachricht) bool { return n.Typ == "fehler" }); n.Code != kern.CodeNichtBerechtigt {
		t.Errorf("code %q erwartet, %q bekommen", kern.CodeNichtBerechtigt, n.Code)
	}
}

// TestProtokollUnbekannteNachricht: unbekannte Typen werden ignoriert, nicht
// beantwortet — und ändern nichts.
func TestProtokollUnbekannteNachricht(t *testing.T) {
	dienst := aufstellen(t)
	anke := verbinden(t, dienst)
	n := anke.anmelden(1, "1111")
	vorher := n.Stand

	anke.schicken(map[string]any{"typ": "gibt_es_nicht", "platz": 1})
	anke.schicken(map[string]any{"typ": kern.AktionSitzungEroeffnen})

	nachher := anke.warte(func(n nachricht) bool { return n.Sitzung.Zustand == string(kern.SitzungLaufend) })
	if nachher.Stand != vorher+1 {
		t.Errorf("die unbekannte nachricht hat den stand verändert: %d -> %d", vorher, nachher.Stand)
	}
}

// TestSeitenWerdenAusgeliefert: jede Rolle hat ihre Seite.
func TestSeitenWerdenAusgeliefert(t *testing.T) {
	dienst := aufstellen(t)

	for _, pfad := range []string{"/", "/namensschild?platz=1", "/dolmetscher", "/testumgebung", "/vorabcheck"} {
		antwort, err := http.Get(dienst.URL + pfad)
		if err != nil {
			t.Fatalf("%s: %v", pfad, err)
		}
		antwort.Body.Close()
		if antwort.StatusCode != http.StatusOK {
			t.Errorf("%s: HTTP %d erwartet, %d bekommen", pfad, http.StatusOK, antwort.StatusCode)
		}
		if typ := antwort.Header.Get("Content-Type"); !strings.HasPrefix(typ, "text/html") {
			t.Errorf("%s: text/html erwartet, %q bekommen", pfad, typ)
		}
	}
}

// TestVorabcheckUeberHTTP: der Bericht kommt als JSON, und während einer
// laufenden Sitzung wird abgelehnt.
func TestVorabcheckUeberHTTP(t *testing.T) {
	dienst := aufstellen(t)

	antwort, err := http.Post(dienst.URL+"/vorabcheck", "", nil)
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	defer antwort.Body.Close()
	if antwort.StatusCode != http.StatusOK {
		t.Fatalf("HTTP 200 erwartet, %d bekommen", antwort.StatusCode)
	}

	var bericht vorabcheck.Bericht
	if err := json.NewDecoder(antwort.Body).Decode(&bericht); err != nil {
		t.Fatalf("bericht nicht lesbar: %v", err)
	}
	if !bericht.Bereit || len(bericht.Punkte) == 0 {
		t.Errorf("bereit erwartet, bekommen: %+v", bericht)
	}

	// Läuft die Sitzung, ist der Check gesperrt — er bewegt Kameras.
	anke := verbinden(t, dienst)
	anke.anmelden(1, "1111")
	anke.schicken(map[string]any{"typ": kern.AktionSitzungEroeffnen})
	anke.warte(func(n nachricht) bool { return n.Sitzung.Zustand == string(kern.SitzungLaufend) })

	gesperrt, err := http.Post(dienst.URL+"/vorabcheck", "", nil)
	if err != nil {
		t.Fatalf("vorabcheck: %v", err)
	}
	defer gesperrt.Body.Close()
	if gesperrt.StatusCode != http.StatusConflict {
		t.Errorf("HTTP 409 erwartet, %d bekommen", gesperrt.StatusCode)
	}
}
