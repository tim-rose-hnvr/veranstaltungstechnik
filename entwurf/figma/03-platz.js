// Phase 3 — Platz-Kachel. Die tragende Komponente: sie trägt die Identität
// des Sitzplatzes und alle Zustände, die der Saal auf einen Blick braucht.
//
// Achsen: Zustand (5) × Ich (2) × Widerspruch (2) = 20 Varianten.
// Widerspruch ist eine eigene Achse und kein Zustand, weil er sich mit jedem
// Zustand überlagert: wer widersprochen hat, kann trotzdem sprechen — das
// Mikrofon geht auf, nur die Kamera bleibt stehen.
const V = async (id) => await figma.variables.getVariableByIdAsync(id);
const fuellung = async (id) => [figma.variables.setBoundVariableForPaint(
  { type: 'SOLID', color: { r: 0, g: 0, b: 0 } }, 'color', await V(id))];

const seite = await figma.getNodeByIdAsync('4:5');            // Platz
await figma.setCurrentPageAsync(seite);

for (const [fam, sch] of [['IBM Plex Sans','Regular'], ['IBM Plex Sans','SemiBold'],
                          ['IBM Plex Mono','Medium']]) {
  await figma.loadFontAsync({ family: fam, style: sch });
}

const F = {
  grund: 'VariableID:2:25', blatt: 'VariableID:2:26', vertieft: 'VariableID:2:27',
  linie: 'VariableID:2:28', stark: 'VariableID:2:30', leise: 'VariableID:2:31',
  haus: 'VariableID:2:33', mikro: 'VariableID:2:34', wort: 'VariableID:2:35',
  frei: 'VariableID:2:36', achtung: 'VariableID:2:37'
};

// Zustand → [Randfarbe, Randstärke, gestrichelt, Lagetext, Lagefarbe]
const ZUSTAENDE = {
  'frei':      [F.linie, 1, true,  'nicht besetzt', F.leise],
  'besetzt':   [F.linie, 1, false, 'am Platz',      F.leise],
  'gemeldet':  [F.linie, 1, false, 'gemeldet',      F.leise],
  'hat-Wort':  [F.wort,  2, false, 'hat das Wort',  F.wort],
  'spricht':   [F.mikro, 2, false, 'spricht',       F.mikro]
};

async function kachel(zustand, ich, widerspruch) {
  const [randId, randStaerke, gestrichelt, lageText, lageId] = ZUSTAENDE[zustand];

  const k = figma.createComponent();
  k.name = 'Zustand=' + zustand + ', Ich=' + ich + ', Widerspruch=' + widerspruch;
  k.layoutMode = 'VERTICAL';
  k.primaryAxisSizingMode = 'FIXED';
  k.counterAxisSizingMode = 'FIXED';
  k.resize(168, 104);
  k.itemSpacing = 4;
  k.paddingTop = 12; k.paddingBottom = 12;
  k.paddingLeft = 14; k.paddingRight = 14;
  k.cornerRadius = 8;
  k.fills = await fuellung(zustand === 'frei' ? F.grund : F.blatt);
  k.strokes = await fuellung(randId);
  k.strokeWeight = randStaerke;
  if (gestrichelt) k.dashPattern = [4, 4];

  // Der eigene Platz bekommt einen inneren Rahmen statt einer eigenen Farbe —
  // Farbe ist für Zustände reserviert.
  if (ich === 'ja') {
    k.effects = [{
      type: 'INNER_SHADOW', color: { r: 0.10, g: 0.12, b: 0.11, a: 1 },
      offset: { x: 0, y: 0 }, radius: 0, spread: 2, visible: true, blendMode: 'NORMAL'
    }];
  }

  const kopf = figma.createAutoLayout('HORIZONTAL', { name: 'Kopf', itemSpacing: 6 });
  k.appendChild(kopf);
  kopf.fills = []; kopf.layoutSizingHorizontal = 'FILL';
  kopf.counterAxisAlignItems = 'CENTER';

  const nr = figma.createText();
  nr.fontName = { family: 'IBM Plex Mono', style: 'Medium' };
  nr.characters = '07';
  nr.fontSize = 20;
  nr.fills = await fuellung(zustand === 'spricht' ? F.mikro : F.stark);
  kopf.appendChild(nr);

  if (ich === 'ja') {
    const marke = figma.createText();
    marke.fontName = { family: 'IBM Plex Sans', style: 'SemiBold' };
    marke.characters = 'ICH';
    marke.fontSize = 10;
    marke.letterSpacing = { unit: 'PIXELS', value: 1 };
    marke.fills = await fuellung(F.leise);
    kopf.appendChild(marke);
  }

  const wer = figma.createText();
  wer.fontName = { family: 'IBM Plex Sans', style: 'SemiBold' };
  wer.characters = zustand === 'frei' ? 'Platz 7' : 'Jonas Öztürk';
  wer.fontSize = 15;
  wer.fills = await fuellung(zustand === 'frei' ? F.leise : F.stark);
  k.appendChild(wer);
  wer.layoutSizingHorizontal = 'FILL';
  wer.textTruncation = 'ENDING';
  wer.maxLines = 1;

  const lage = figma.createText();
  lage.fontName = { family: 'IBM Plex Sans', style: 'Regular' };
  // Widerspruch überlagert die Lage: die Kamera bleibt stehen, das Mikrofon
  // geht trotzdem auf. Das muss dastehen, sonst sucht die Technik den Fehler.
  lage.characters = widerspruch === 'ja' ? (lageText + ' · kein Bild') : lageText;
  lage.fontSize = 13;
  lage.fills = await fuellung(widerspruch === 'ja' ? F.achtung : lageId);
  k.appendChild(lage);
  lage.layoutSizingHorizontal = 'FILL';

  return k;
}

const varianten = [];
for (const zustand of Object.keys(ZUSTAENDE)) {
  for (const ich of ['nein', 'ja']) {
    for (const widerspruch of ['nein', 'ja']) {
      varianten.push(await kachel(zustand, ich, widerspruch));
    }
  }
}

const satz = figma.combineAsVariants(varianten, seite);
satz.name = 'Platz';
satz.description =
  'Der Sitzplatz im Saalplan. Zustand kommt aus dem Kern, nie aus der Oberfläche.\n\n' +
  'Widerspruch ist eine eigene Achse, kein Zustand: wer der Aufzeichnung ' +
  'widersprochen hat, kann trotzdem sprechen — das Mikrofon geht auf, nur die ' +
  'Kamera bleibt stehen. Deshalb überlagert er jeden Zustand.\n\n' +
  'Zustand hängt nie allein an der Farbe: jede Kachel trägt zusätzlich Text.';

// Nach combineAsVariants stapeln die Varianten auf (0,0) — von Hand rastern.
satz.layoutMode = 'NONE';
const SPALTEN = 4, BREITE = 168, HOEHE = 104, LUFT = 20;
satz.children.forEach((v, i) => {
  v.x = (i % SPALTEN) * (BREITE + LUFT);
  v.y = Math.floor(i / SPALTEN) * (HOEHE + LUFT);
});
satz.resize(SPALTEN * (BREITE + LUFT) - LUFT + 48,
            Math.ceil(varianten.length / SPALTEN) * (HOEHE + LUFT) - LUFT + 48);
satz.x = 80; satz.y = 200;
satz.fills = await fuellung(F.grund);
satz.paddingTop = 24; satz.paddingLeft = 24;

return {
  komponentensatz: satz.id,
  variantenAnzahl: satz.children.length,
  achsen: Object.keys(satz.componentPropertyDefinitions),
  createdNodeIds: varianten.map(v => v.id)
};
