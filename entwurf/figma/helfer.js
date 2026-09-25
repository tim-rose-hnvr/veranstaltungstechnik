// ConferenceCue — gemeinsame Helfer für alle use_figma-Bauskripte.
// Diesen Block unverändert an den Anfang jedes Skripts setzen. Er sucht
// Variablen, Textstile und Effektstile über ihren NAMEN, nicht über Kennungen,
// damit neue Tokens ohne Nacharbeit verfügbar sind.
//
// Zwei Fallen, die dieser Block abfängt:
// 1. Farbe an eine Variable binden: den aufgelösten Wert als color mitgeben.
//    Mit color 0,0,0 als Platzhalter zeigt der Renderer sonst Schwarz, wenn
//    dieselbe Variable schon gebunden war.
// 2. resize() auf Text setzt textAutoResize zurück — text() erledigt die
//    Reihenfolge.
const CC = await (async () => {
  const alle = await figma.variables.getLocalVariablesAsync();
  const vars = {}; for (const v of alle) vars[v.name] = v;
  const sammlungen = await figma.variables.getLocalVariableCollectionsAsync();
  const farbSammlung = sammlungen.find(c => c.name === 'Farbe');
  const HELL = farbSammlung.modes.find(m => m.name === 'Hell').modeId;
  const DUNKEL = farbSammlung.modes.find(m => m.name === 'Dunkel').modeId;
  const cache = {};
  async function wert(v, modus) {
    const key = v.id + modus; if (cache[key]) return cache[key];
    let val = v.valuesByMode[modus] ?? Object.values(v.valuesByMode)[0];
    while (val && val.type === 'VARIABLE_ALIAS') {
      const a = await figma.variables.getVariableByIdAsync(val.id);
      val = a.valuesByMode[modus] ?? Object.values(a.valuesByMode)[0];
    }
    return (cache[key] = val);
  }
  function v(name) { const x = vars[name]; if (!x) throw new Error('Variable fehlt: ' + name); return x; }
  async function farbe(name, deckkraft) {
    const x = v(name); const c = await wert(x, HELL);
    const p = { type: 'SOLID', color: { r: c.r, g: c.g, b: c.b } };
    if (deckkraft !== undefined) p.opacity = deckkraft;
    return [figma.variables.setBoundVariableForPaint(p, 'color', x)];
  }
  const stile = {}; for (const s of await figma.getLocalTextStylesAsync()) stile[s.name.replace(/^Schrift\//, '')] = s;
  const effekte = {}; for (const s of await figma.getLocalEffectStylesAsync()) effekte[s.name.replace(/^Ebene\//, '')] = s;
  const schriften = new Set(Object.values(stile).map(s => JSON.stringify(s.fontName)));
  for (const f of ['IBM Plex Sans|Regular', 'IBM Plex Sans|Medium', 'IBM Plex Sans|SemiBold', 'IBM Plex Sans|Bold',
                   'IBM Plex Mono|Regular', 'IBM Plex Mono|Medium', 'Source Serif 4|Regular', 'Source Serif 4|SemiBold'])
    schriften.add(JSON.stringify({ family: f.split('|')[0], style: f.split('|')[1] }));
  await Promise.all([...schriften].map(f => figma.loadFontAsync(JSON.parse(f)).catch(() => null)));

  // Text mit Stil und Farbvariable. breite: Zahl = feste Breite mit Umbruch,
  // 'FILL' = füllt das Auto-Layout-Elternteil (nach dem Anhängen), sonst Hug.
  async function text(eltern, inhalt, stil, farbName, breite) {
    const t = figma.createText();
    if (!stile[stil]) throw new Error('Textstil fehlt: ' + stil);
    await t.setTextStyleIdAsync(stile[stil].id);
    t.characters = inhalt;
    t.fills = await farbe(farbName || 'text/stark');
    if (eltern) eltern.appendChild(t);
    if (typeof breite === 'number') { t.resize(breite, t.height); t.textAutoResize = 'HEIGHT'; }
    else if (breite === 'FILL') { t.layoutSizingHorizontal = 'FILL'; t.textAutoResize = 'HEIGHT'; }
    return t;
  }
  // Auto-Layout-Rahmen ohne Füllung.
  function box(eltern, name, richtung, abstand, props) {
    const f = figma.createAutoLayout(richtung || 'VERTICAL', Object.assign({ name, itemSpacing: abstand ?? 0 }, props || {}));
    f.fills = [];
    if (eltern) eltern.appendChild(f);
    return f;
  }
  // Innenabstand: pad(f, alle) | pad(f, senkrecht, waagerecht) | pad(f, oben, rechts, unten, links)
  function pad(f, a, b, c, d) {
    if (b === undefined) b = a; if (c === undefined) c = a; if (d === undefined) d = b;
    f.paddingTop = a; f.paddingRight = b; f.paddingBottom = c; f.paddingLeft = d; return f;
  }
  function radius(f, name) {
    const x = v('radius/' + name);
    for (const e of ['topLeftRadius', 'topRightRadius', 'bottomLeftRadius', 'bottomRightRadius']) f.setBoundVariable(e, x);
    return f;
  }
  // Nur eine Kante zeichnen: kante(f, 'unten', 'linie/normal')
  async function kante(f, seite, farbName, staerke) {
    f.strokes = await farbe(farbName || 'linie/normal');
    f.strokeWeight = staerke || 1;
    f.strokeTopWeight = seite === 'oben' ? (staerke || 1) : 0;
    f.strokeBottomWeight = seite === 'unten' ? (staerke || 1) : 0;
    f.strokeLeftWeight = seite === 'links' ? (staerke || 1) : 0;
    f.strokeRightWeight = seite === 'rechts' ? (staerke || 1) : 0;
    return f;
  }
  async function rand(f, farbName, staerke) { f.strokes = await farbe(farbName || 'linie/normal'); f.strokeWeight = staerke || 1; f.strokeAlign = 'INSIDE'; return f; }
  function ebene(f, name) { if (effekte[name]) f.effectStyleId = effekte[name].id; return f; }
  function luft(eltern) { const f = figma.createFrame(); f.name = 'Luft'; f.fills = []; eltern.appendChild(f); f.layoutSizingHorizontal = 'FILL'; f.layoutSizingVertical = 'FILL'; return f; }
  // Instanz aus einem Komponentensatz: variante(SATZ_ID, 'Art=Stark, Größe=Gross')
  async function variante(satzId, name, props) {
    const satz = await figma.getNodeByIdAsync(satzId);
    if (!satz) throw new Error('Komponente fehlt: ' + satzId);
    let quelle = satz;
    if (satz.type === 'COMPONENT_SET') {
      quelle = satz.children.find(c => c.name === name);
      if (!quelle) throw new Error('Variante fehlt: ' + name + ' in ' + satz.name + ' — vorhanden: ' + satz.children.map(c => c.name).join(' | '));
    }
    const i = quelle.createInstance();
    if (props) {
      const defs = satz.type === 'COMPONENT_SET' || (satz.type === 'COMPONENT' && satz.parent?.type !== 'COMPONENT_SET') ? satz.componentPropertyDefinitions : {};
      const setzen = {};
      for (const [k, wert] of Object.entries(props)) {
        const schluessel = Object.keys(defs).find(d => d === k || d.startsWith(k + '#'));
        if (!schluessel) throw new Error('Eigenschaft fehlt: ' + k + ' — vorhanden: ' + Object.keys(defs).join(', '));
        setzen[schluessel] = wert;
      }
      i.setProperties(setzen);
    }
    return i;
  }
  return { vars, v, farbe, stile, effekte, text, box, pad, radius, kante, rand, ebene, luft, variante, HELL, DUNKEL, farbSammlung };
})();
