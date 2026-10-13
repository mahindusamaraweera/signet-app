/* Signet dashboard layer — visual effects only. Runs on pages whose <body> has .sg-dashboard.
   Never changes data: animated numbers always end on the exact original text.

   Port of the original's public/resources/assets/js/signet-dashboard.js. The
   effect code is the original's, unchanged, except that everything it starts
   (timers, animation frames, observers, listeners, injected nodes) goes
   through `track` so initSignetDashboard() can return a cleanup function:
   the SPA tears a dashboard down on navigation instead of unloading the page.
   Call it once the dashboard's data has rendered (the original ran after the
   server-rendered page loaded). */
export function initSignetDashboard() {
  'use strict';
  var destroyed = false;
  var stops = [], nodes = [];
  var track = {
    interval: function (fn, ms) { var id = setInterval(fn, ms); stops.push(function () { clearInterval(id); }); return id; },
    timeout: function (fn, ms) { var id = setTimeout(fn, ms); stops.push(function () { clearTimeout(id); }); return id; },
    raf: function (fn) { return requestAnimationFrame(function (ts) { if (!destroyed) fn(ts); }); },
    observer: function (o) { stops.push(function () { o.disconnect(); }); return o; },
    on: function (target, type, fn) { target.addEventListener(type, fn); stops.push(function () { target.removeEventListener(type, fn); }); },
    node: function (n) { nodes.push(n); return n; }
  };
  function cleanup() {
    destroyed = true;
    stops.forEach(function (f) { f(); });
    nodes.forEach(function (n) { if (n.parentNode) n.parentNode.removeChild(n); });
  }
  if (!document.body || !document.body.classList.contains('sg-dashboard')) return cleanup;

  var reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  var fine = window.matchMedia && window.matchMedia('(hover: hover) and (pointer: fine)').matches;
  var content = document.querySelector('.content');

  /* live clock in the top bar */
  var slot = document.querySelector('.navbar-top > .container-fluid > .d-flex > .d-flex:first-child');
  if (slot) {
    var clock = document.createElement('span');
    clock.className = 'sg-clock';
    clock.setAttribute('aria-hidden', 'true');
    slot.appendChild(track.node(clock));
    var pad = function (n) { return (n < 10 ? '0' : '') + n; };
    var tick = function () {
      var d = new Date();
      clock.textContent = pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
    };
    tick();
    track.interval(function () { if (!document.hidden) tick(); }, 1000);
  }

  /* ---------- stat icons: one duotone icon + colour per stat, picked from its label ---------- */
  var SVG_OPEN = '<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" xmlns="http://www.w3.org/2000/svg">';
  var ICONS = {
    token: SVG_OPEN + '<ellipse cx="12" cy="6.5" rx="7" ry="3" fill="currentColor" fill-opacity=".2"/><path d="M5 6.5v4.5c0 1.7 3.1 3 7 3s7-1.3 7-3V6.5"/><path d="M5 11v4.5c0 1.7 3.1 3 7 3s7-1.3 7-3V11"/></svg>',
    wallet: SVG_OPEN + '<path d="M4 8.5A2.5 2.5 0 0 1 6.5 6H18a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H6.5A2.5 2.5 0 0 1 4 16.5z" fill="currentColor" fill-opacity=".14"/><path d="M4 8.5V7a2 2 0 0 1 2-2h9.5"/><circle cx="16.2" cy="13" r="1.4" fill="currentColor"/></svg>',
    director: SVG_OPEN + '<path d="M4.5 18h15"/><path d="M5.2 17.5 4 8.8l4.6 3.6L12 5.8l3.4 6.6L20 8.8l-1.2 8.7z" fill="currentColor" fill-opacity=".16"/><circle cx="4" cy="8.6" r="1" fill="currentColor"/><circle cx="12" cy="5.4" r="1" fill="currentColor"/><circle cx="20" cy="8.6" r="1" fill="currentColor"/></svg>',
    earn: SVG_OPEN + '<path d="M3.5 19.5h17"/><path d="M4 16.5l5-5 3.5 3.5L20 7.5"/><path d="M15 7.5h5v5"/></svg>',
    pool: SVG_OPEN + '<path d="M12 3.5c3.6 4.2 6 7 6 10a6 6 0 0 1-12 0c0-3 2.4-5.8 6-10z" fill="currentColor" fill-opacity=".16"/><path d="M9.3 14.2a2.7 2.7 0 0 0 2.7 2.6"/></svg>',
    share: SVG_OPEN + '<path d="M12 3.5v8.5h8.5" fill="currentColor" fill-opacity=".16"/><path d="M20.3 15.2A8.5 8.5 0 1 1 8.8 3.9"/></svg>',
    portion: SVG_OPEN + '<circle cx="7.5" cy="7.5" r="2.6" fill="currentColor" fill-opacity=".16"/><circle cx="16.5" cy="16.5" r="2.6" fill="currentColor" fill-opacity=".16"/><path d="M18.5 5.5l-13 13"/></svg>',
    income: SVG_OPEN + '<path d="M4 17a8 8 0 0 1 16 0" fill="currentColor" fill-opacity=".14"/><path d="M12 17l4-5"/><circle cx="12" cy="17" r="1.3" fill="currentColor"/><path d="M3.5 20.5h17"/></svg>'
  };
  function statKind(label) {
    label = (label || '').toLowerCase();
    if (label.indexOf('income limit') > -1) return 'income';
    if (label.indexOf('global director') > -1) return 'director';
    if (label.indexOf('wallet') > -1) return 'wallet';
    if (label.indexOf('my token') > -1) return 'token';
    if (label.indexOf('my earn') > -1) return 'earn';
    if (label.indexOf('direct share pool') > -1) return 'pool';
    if (label.indexOf('all share') > -1) return 'share';
    if (label.indexOf('share portion') > -1) return 'portion';
    return null;
  }
  var shapes = document.querySelectorAll('.card-body .icon-shape');
  for (var s = 0; s < shapes.length; s++) {
    var body = shapes[s].closest('.card-body');
    var head = body && body.querySelector('.h6, h2.h5, h2');
    var kind = head ? statKind(head.textContent) : null;
    if (!kind) continue;
    shapes[s].setAttribute('data-kind', kind);
    var old = shapes[s].querySelector('svg');
    if (old) old.outerHTML = ICONS[kind];
  }

  /* ---------- mining card: cartoon miner + Signet coins that burst out and tumble (visual only) ---------- */
  (function mining() {
    var tokenEl = document.getElementById('miningToken');
    var card = tokenEl && tokenEl.closest('.card');
    var cardBody = card && card.querySelector('.card-body');
    if (!cardBody) return;
    card.classList.add('sg-mining');

    var VBW = 720, GY = 232;                       // scene width / ground line, in scene units
    var CYCLE = 1.8, IMP = 0.52;                   // seconds per swing, phase of the strike
    var IMPACT = { x: 363, y: 143 };               // where the pickaxe meets the rock
    var SOURCE = { x: 352, y: 146 };               // the coin embedded in the rock
    var COIN_URL = '/resources/assets/img/brand/signet-coin.webp';

    var scene = document.createElement('div');
    scene.className = 'sg-mine';
    scene.setAttribute('aria-hidden', 'true');
    scene.innerHTML =
      '<div class="sg-mine__bg"></div>' +
      '<div class="sg-mine__label"><b></b><span>STANDBY</span></div>' +
      '<div class="sg-mine__gauge"><span><i></i></span><em>DEPTH</em></div>' +
      '<svg class="sg-mine__svg" viewBox="0 0 720 272" preserveAspectRatio="xMidYMid slice" xmlns="http://www.w3.org/2000/svg">' +
        '<defs>' +
          '<linearGradient id="sgRockG" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#3a4049"/><stop offset="1" stop-color="#1d2026"/></linearGradient>' +
          '<radialGradient id="sgGoldGlow"><stop offset="0" stop-color="#FFD470" stop-opacity=".9"/><stop offset=".5" stop-color="#F5B340" stop-opacity=".32"/><stop offset="1" stop-color="#F5B340" stop-opacity="0"/></radialGradient>' +
          '<linearGradient id="sgGround" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#0d141b"/><stop offset="1" stop-color="#05090d"/></linearGradient>' +
          '<linearGradient id="sgSkin" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#F3BB92"/><stop offset="1" stop-color="#CE8D64"/></linearGradient>' +
          '<linearGradient id="sgSkinD" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#C98B62"/><stop offset="1" stop-color="#9F6844"/></linearGradient>' +
          '<linearGradient id="sgHair" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#83512A"/><stop offset="1" stop-color="#41270F"/></linearGradient>' +
          '<linearGradient id="sgHat" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#FFE770"/><stop offset="1" stop-color="#E3A100"/></linearGradient>' +
          '<linearGradient id="sgShirt" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#2E4870"/><stop offset="1" stop-color="#4E72A2"/></linearGradient>' +
          '<linearGradient id="sgVest" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#D9640A"/><stop offset=".55" stop-color="#FF9A2E"/><stop offset="1" stop-color="#F27F14"/></linearGradient>' +
          '<linearGradient id="sgReflect" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#9AA6B2"/><stop offset=".5" stop-color="#F2F6FA"/><stop offset="1" stop-color="#B5BFCA"/></linearGradient>' +
          '<linearGradient id="sgBoot" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#5A4739"/><stop offset="1" stop-color="#221811"/></linearGradient>' +
          '<linearGradient id="sgGlove" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#9A7048"/><stop offset="1" stop-color="#5A3E26"/></linearGradient>' +
          '<linearGradient id="sgWood" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#D49A84"/><stop offset=".55" stop-color="#A66655"/><stop offset="1" stop-color="#7C4A3D"/></linearGradient>' +
          '<linearGradient id="sgSteel" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#EDE6F7"/><stop offset=".5" stop-color="#B4A9C8"/><stop offset="1" stop-color="#8E82A6"/></linearGradient>' +
          '<linearGradient id="sgBeam" gradientUnits="userSpaceOnUse" x1="34" y1="-91" x2="200" y2="-91"><stop offset="0" stop-color="#FFF4C8" stop-opacity=".42"/><stop offset="1" stop-color="#FFF4C8" stop-opacity="0"/></linearGradient>' +
        '</defs>' +
        '<g transform="translate(-25 12)">' +
        '<rect x="-1000" y="232" width="3000" height="90" fill="url(#sgGround)"/>' +
        '<path d="M-1000 232H2000" stroke="rgba(255,255,255,.14)"/>' +
        '<ellipse id="sgShadow" cx="250" cy="234" rx="58" ry="5" fill="rgba(0,0,0,.55)"/>' +
        '<g id="sgRock">' +
          '<circle id="sgGlow" cx="352" cy="146" r="52" fill="url(#sgGoldGlow)" opacity="0"/>' +
          '<path d="M306 232 L312 208 Q318 180 332 162 Q338 138 358 122 Q386 102 418 108 Q452 98 486 114 Q522 134 540 174 Q556 200 562 232 Z" fill="url(#sgRockG)" stroke="rgba(255,255,255,.07)"/>' +
          '<path d="M332 162 L358 122 L376 160 Z" fill="#4a505b" opacity=".5"/>' +
          '<path d="M418 108 L486 114 L456 160 Z" fill="#454b55" opacity=".45"/>' +
          '<path d="M386 190 L436 176 L466 232 L396 232 Z" fill="#14171b" opacity=".5"/>' +
          '<path d="M376 160 L396 190 M456 160 L436 176 M486 114 L506 150" stroke="#12151a" stroke-width="2" fill="none"/>' +
          '<image id="sgCoinSrc" x="329" y="123" width="46" height="46" preserveAspectRatio="xMidYMid meet"/>' +
          '<path d="M352 152 Q364 146 378 152 L384 182 L340 186 Q348 168 352 152 Z" fill="url(#sgRockG)"/>' +
        '</g>' +
        '<g id="sgMiner" transform="translate(250 168)"></g>' +
        '</g>' +
      '</svg>' +
      '<canvas class="sg-mine__fx"></canvas>';
    cardBody.insertBefore(track.node(scene), cardBody.firstChild);

    var labelText = scene.querySelector('.sg-mine__label span');
    var gauge = scene.querySelector('.sg-mine__gauge i');
    var statusEl = document.getElementById('statusBadge');
    var barEl = document.getElementById('progressBar');
    var minerG = scene.querySelector('#sgMiner');
    var shadowEl = scene.querySelector('#sgShadow');
    var rockG = scene.querySelector('#sgRock');
    var glowEl = scene.querySelector('#sgGlow');
    var coinSrcEl = scene.querySelector('#sgCoinSrc');
    var cv = scene.querySelector('.sg-mine__fx');
    var ctx = cv.getContext('2d');
    coinSrcEl.setAttribute('href', COIN_URL);

    /* ---- the miner: built once from vector parts, joints solved every frame ---- */
    var NS = 'http://www.w3.org/2000/svg';
    function el(tag, attrs, parent) {
      var e = document.createElementNS(NS, tag);
      for (var k in attrs) e.setAttribute(k, attrs[k]);
      if (parent) parent.appendChild(e);
      return e;
    }
    function limb(parent, main, outline, hl) {
      return {
        o: el('line', { stroke: outline, 'stroke-linecap': 'round' }, parent),
        m: el('line', { stroke: main, 'stroke-linecap': 'round' }, parent),
        h: el('line', { stroke: hl, 'stroke-linecap': 'round', opacity: '.5' }, parent)
      };
    }
    function setLine(l, x1, y1, x2, y2, w) {
      l.setAttribute('x1', x1.toFixed(2)); l.setAttribute('y1', y1.toFixed(2));
      l.setAttribute('x2', x2.toFixed(2)); l.setAttribute('y2', y2.toFixed(2));
      l.setAttribute('stroke-width', w.toFixed(2));
    }
    function setLimb(l, a, b, w) {
      var dx = b.x - a.x, dy = b.y - a.y, d = Math.sqrt(dx * dx + dy * dy) || 1, nx = -dy / d, ny = dx / d;
      if (nx - ny < 0) { nx = -nx; ny = -ny; }
      var off = w * .2;
      setLine(l.o, a.x, a.y, b.x, b.y, w + 2.2);
      setLine(l.m, a.x, a.y, b.x, b.y, w);
      setLine(l.h, a.x + nx * off, a.y + ny * off, b.x + nx * off, b.y + ny * off, w * .24);
    }
    function ik(sx, sy, tx, ty, l1, l2, forward) {
      var dx = tx - sx, dy = ty - sy, d = Math.sqrt(dx * dx + dy * dy), max = l1 + l2 - .01;
      if (d > max) { tx = sx + dx / d * max; ty = sy + dy / d * max; dx = tx - sx; dy = ty - sy; d = max; }
      if (d < .5) d = .5;
      var a = (l1 * l1 - l2 * l2 + d * d) / (2 * d), h = Math.sqrt(Math.max(0, l1 * l1 - a * a));
      var mx = sx + dx / d * a, my = sy + dy / d * a, nx = -dy / d, ny = dx / d;
      var e1 = { x: mx + nx * h, y: my + ny * h }, e2 = { x: mx - nx * h, y: my - ny * h };
      var e = forward ? (e1.x > e2.x ? e1 : e2) : (e1.y > e2.y ? e1 : e2);
      return { e: e, h: { x: tx, y: ty } };
    }
    function lerpPt(a, b, t) { return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t }; }

    var defs = scene.querySelector('defs');
    defs.insertAdjacentHTML('beforeend',
      '<linearGradient id="sgSteel2" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#F6F2FB"/><stop offset=".32" stop-color="#C8BEDA"/><stop offset=".68" stop-color="#8D82A7"/><stop offset="1" stop-color="#564D70"/></linearGradient>' +
      '<linearGradient id="sgSteelEdge" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#FFFFFF"/><stop offset="1" stop-color="#BDB2D2"/></linearGradient>' +
      '<linearGradient id="sgSocket" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#6c6480"/><stop offset=".5" stop-color="#3f394f"/><stop offset="1" stop-color="#26212f"/></linearGradient>' +
      '<linearGradient id="sgWood2" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#E2AE98"/><stop offset=".45" stop-color="#B0705E"/><stop offset="1" stop-color="#74463A"/></linearGradient>' +
      '<linearGradient id="sgLeather" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#5a4030"/><stop offset="1" stop-color="#2a1c12"/></linearGradient>' +
      '<linearGradient id="sgFaceShade" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#3a1c08" stop-opacity=".55"/><stop offset=".45" stop-color="#3a1c08" stop-opacity=".12"/><stop offset="1" stop-color="#3a1c08" stop-opacity="0"/></linearGradient>' +
      '<linearGradient id="sgBeard" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#7d5030"/><stop offset=".6" stop-color="#4f2f16"/><stop offset="1" stop-color="#33200e"/></linearGradient>' +
      '<radialGradient id="sgLens"><stop offset="0" stop-color="#FFFFFF"/><stop offset=".5" stop-color="#FFF3B8"/><stop offset="1" stop-color="#FFD54A" stop-opacity="0"/></radialGradient>' +
      '<clipPath id="sgTorsoClip"><path d="M-12 3 C-16 -8 -16 -22 -14 -34 C-12 -44 -8 -49 -2 -50 L7 -50 C12 -46 15 -40 15 -30 C15 -20 14 -10 12 3 Z"/></clipPath>' +
      '<clipPath id="sgFaceClip"><path d="M-10 -80 C-11 -90 -2 -94 8 -93 C18 -92 23 -86 23 -79 L22.5 -77 C23.6 -76 23.2 -74.6 22.6 -73.6 C24 -70.4 27.6 -67.4 27.4 -65.6 C27.2 -63.8 25 -63.2 23.6 -63 C23.2 -62.2 23.4 -61.2 23.8 -60.4 C24.2 -59.4 23.6 -58.6 22.6 -58.2 C21 -55.2 17 -52.4 10 -51.4 C1 -51.6 -5 -55 -8 -62 C-10.4 -68 -10.6 -74 -10 -80 Z"/></clipPath>');

    /* boots: tall leather work boots with laces, cuffs tucked in and a lugged sole */
    function boot(parent) {
      var g = el('g', {}, parent);
      g.innerHTML =
        '<path d="M-7.6 -17 L7.6 -17 L9 -1 Q10.4 3.4 17 5.4 Q23.4 7.4 23.4 11.6 L-9.6 11.6 Q-10.6 2 -7.6 -17 Z" fill="url(#sgBoot)" stroke="#100b07" stroke-width=".9" stroke-linejoin="round"/>' +
        '<path d="M-8 -17 L8 -17 L8.4 -12.6 L-8.4 -12.6 Z" fill="#26324a" stroke="#11182a" stroke-width=".7"/>' +
        '<path d="M-9.6 8.2 L23.4 8.2 L23.4 12.6 Q23.4 13.6 22.4 13.6 L-9 13.6 Q-10 13.6 -10 12.6 Z" fill="#15100c"/>' +
        '<path d="M-7 13.6 v1.6 M-3.4 13.6 v1.6 M0.2 13.6 v1.6 M3.8 13.6 v1.6 M7.4 13.6 v1.6 M11 13.6 v1.6 M14.6 13.6 v1.6 M18.2 13.6 v1.6" stroke="#0a0705" stroke-width="1" stroke-linecap="round"/>' +
        '<path d="M-9.6 8.2 L23.4 8.2" stroke="rgba(255,255,255,.14)" stroke-width=".8"/>' +
        '<path d="M13 5.8 Q21.4 7 22.6 10.4" stroke="rgba(255,255,255,.26)" fill="none" stroke-linecap="round"/>' +
        '<path d="M-1.4 -9.6 L6 -11 M-1.2 -6 L7.4 -7.4 M-0.8 -2.4 L8.2 -3.8 M-0.4 1 L9 -0.4" stroke="rgba(238,226,200,.7)" stroke-width="1.1" fill="none" stroke-linecap="round"/>' +
        '<circle cx="5.6" cy="-10.6" r=".9" fill="#c9b98f"/><circle cx="7" cy="-7" r=".9" fill="#c9b98f"/><circle cx="7.8" cy="-3.4" r=".9" fill="#c9b98f"/>' +
        '<path d="M-6 -12 Q-8 -2 -8 8" stroke="rgba(0,0,0,.35)" stroke-width="1.2" fill="none"/>';
      return g;
    }
    /* gloved hand: wrist cuff, palm, four fingers wrapped over the handle, thumb */
    function glove(parent, sc) {
      var g = el('g', {}, parent);
      g.innerHTML =
        '<g transform="scale(' + sc + ')">' +
          '<rect x="-10.6" y="-5.6" width="6.4" height="11.2" rx="2.2" fill="#2b1f15" stroke="#140d07" stroke-width=".6"/>' +
          '<path d="M-5 -5.2 Q3 -6.4 7.4 -4.4 L7.4 4.4 Q3 6.4 -5 5.2 Z" fill="url(#sgGlove)" stroke="#231509" stroke-width=".7"/>' +
          '<rect x="-1.2" y="-6.6" width="3.2" height="13.2" rx="1.6" fill="#8a6440" stroke="#231509" stroke-width=".6"/>' +
          '<rect x="1.8" y="-6.8" width="3.2" height="13.6" rx="1.6" fill="#94693f" stroke="#231509" stroke-width=".6"/>' +
          '<rect x="4.8" y="-6.4" width="3.2" height="12.8" rx="1.6" fill="#8a6440" stroke="#231509" stroke-width=".6"/>' +
          '<path d="M-3 -5 Q0 -8.4 4 -7" stroke="rgba(255,235,205,.32)" stroke-width="1" fill="none" stroke-linecap="round"/>' +
        '</g>';
      return g;
    }

    var minerRoot = scene.querySelector('#sgMiner');
    var legBackG = el('g', {}, minerRoot);
    var legFrontG = el('g', {}, minerRoot);
    var bThigh = limb(legBackG, '#27344D', '#121a2b', '#566b8f'), bShin = limb(legBackG, '#27344D', '#121a2b', '#566b8f');
    var bKnee = el('ellipse', { rx: '5.4', ry: '7.4', fill: '#354769', opacity: '.8' }, legBackG);
    var bBoot = boot(legBackG);
    var fThigh = limb(legFrontG, '#33435F', '#141c2e', '#6580a8'), fShin = limb(legFrontG, '#33435F', '#141c2e', '#6580a8');
    var fPocket = el('path', { d: 'M-5 -4 L5 -4 L5.6 5 L-5.6 5 Z', fill: '#3b4e6f', stroke: '#141c2e', 'stroke-width': '.7' }, legFrontG);
    var fPocketLine = el('path', { d: 'M-5.2 -1 L5.4 -1', stroke: 'rgba(255,255,255,.22)', 'stroke-width': '.7' }, legFrontG);
    var fKnee = el('ellipse', { rx: '5.8', ry: '8', fill: '#43587f', opacity: '.85' }, legFrontG);
    var fWrink = el('path', { d: '', stroke: 'rgba(8,12,24,.5)', 'stroke-width': '.9', fill: 'none', 'stroke-linecap': 'round' }, legFrontG);
    var fBoot = boot(legFrontG);

    var torsoG = el('g', { id: 'sgTorso' }, minerRoot);
    var farArmG = el('g', {}, torsoG);
    var bodyG = el('g', { id: 'sgBody' }, torsoG);
    bodyG.innerHTML =
      '<path d="M-15 -6 C-24 -18 -22 -50 -12 -84" stroke="#0e1216" stroke-width="1.3" fill="none" stroke-linecap="round"/>' +
      '<rect x="-19" y="-12" width="8" height="12" rx="2" fill="#20262d" stroke="#0b0e12" stroke-width=".7"/>' +
      '<path d="M-12 3 C-16 -8 -16 -22 -14 -34 C-12 -44 -8 -49 -2 -50 L7 -50 C12 -46 15 -40 15 -30 C15 -20 14 -10 12 3 Z" fill="url(#sgShirt)" stroke="#111c30" stroke-width=".9"/>' +
      '<g clip-path="url(#sgTorsoClip)">' +
        '<path d="M-14 3 L-14 -47 L-5 -49 L1 -30 L3 3 Z" fill="url(#sgVest)"/>' +
        '<path d="M3 3 L1 -30 L-5 -49 L16 -49 L16 3 Z" fill="url(#sgVest)" opacity=".94"/>' +
        '<path d="M-16 -40 L16 -40 L16 -47 L-16 -47 Z" fill="rgba(60,20,0,.18)"/>' +
        '<path d="M-16 -23 Q0 -19.6 16 -23 L16 -17.4 Q0 -14 -16 -17.4 Z" fill="url(#sgReflect)"/>' +
        '<path d="M-16 -38 Q0 -34.6 16 -38 L16 -32.4 Q0 -29 -16 -32.4 Z" fill="url(#sgReflect)"/>' +
        '<path d="M-16 -20.4 Q0 -17 16 -20.4 M-16 -35.4 Q0 -32 16 -35.4" stroke="rgba(0,0,0,.22)" stroke-width=".6" stroke-dasharray="1.4 1.2" fill="none"/>' +
        '<path d="M-6 -49 L-2 -27 L-1 3" stroke="rgba(80,30,0,.5)" stroke-width="1" fill="none"/>' +
        '<path d="M2 -49 L3 3" stroke="rgba(255,225,190,.7)" stroke-width=".9" stroke-dasharray="1.2 1.1" fill="none"/>' +
        '<path d="M-13 -46 Q-14 -20 -11 2" stroke="rgba(120,200,255,.35)" stroke-width="1.2" fill="none"/>' +
        '<path d="M14 -20 Q15 -8 12 3" stroke="rgba(60,20,0,.35)" stroke-width="2" fill="none"/>' +
        '<rect x="6.4" y="-31" width="6.6" height="6.4" rx="1" fill="rgba(90,40,0,.28)" stroke="rgba(70,30,0,.55)" stroke-width=".6"/>' +
        '<path d="M6.4 -29.2 L13 -29.2" stroke="rgba(255,220,180,.45)" stroke-width=".6"/>' +
        '<path d="M-10 -8 Q-3 -10 4 -8.6 M5 -14 Q10 -15 14 -13.4" stroke="rgba(80,30,0,.26)" stroke-width="1" fill="none" stroke-linecap="round"/>' +
      '</g>' +
      '<path d="M-3 -50 L8 -50 L3.4 -40 Z" fill="#5B7FB0" stroke="#22344f" stroke-width=".6"/>' +
      '<path d="M-2 -50 L7 -50 L3.6 -44 Z" fill="url(#sgSkinD)"/>' +
      '<rect x="-14" y="-4.4" width="28" height="7.4" rx="2.2" fill="#2a2018" stroke="#120d08" stroke-width=".6"/>' +
      '<rect x="6" y="-3.6" width="6.6" height="5.8" rx="1.2" fill="#C9B178" stroke="#7d6a3a" stroke-width=".6"/>' +
      '<rect x="7.6" y="-2.2" width="3.4" height="3" rx=".8" fill="#2a2018"/>' +
      '<path d="M-6 -1 q1.6 5 3.6 1.4" stroke="#8e98a4" stroke-width="1.1" fill="none" stroke-linecap="round"/>' +
      '<path d="M-4 -56 L7 -56 L8 -47 L-5 -47 Z" fill="url(#sgSkinD)"/>' +
      '<path d="M-4 -52 Q2 -48.6 8 -52 L8 -47 L-5 -47 Z" fill="rgba(40,16,4,.35)"/>';

    var headG = el('g', { id: 'sgHead' }, torsoG);
    var faceD = 'M-10 -80 C-11 -90 -2 -94 8 -93 C18 -92 23 -86 23 -79 L22.5 -77 C23.6 -76 23.2 -74.6 22.6 -73.6 C24 -70.4 27.6 -67.4 27.4 -65.6 C27.2 -63.8 25 -63.2 23.6 -63 C23.2 -62.2 23.4 -61.2 23.8 -60.4 C24.2 -59.4 23.6 -58.6 22.6 -58.2 C21 -55.2 17 -52.4 10 -51.4 C1 -51.6 -5 -55 -8 -62 C-10.4 -68 -10.6 -74 -10 -80 Z';
    headG.innerHTML =
      '<polygon points="36,-91 340,-160 340,-24" fill="url(#sgBeam)" opacity=".32"/>' +
      '<polygon points="36,-91 340,-138 340,-46" fill="url(#sgBeam)" opacity=".5"/>' +
      '<polygon points="36,-91 340,-118 340,-66" fill="url(#sgBeam)" opacity=".7"/>' +
      '<path d="M-10 -80 C-15 -74 -15 -63 -9 -57 L-5 -60 C-8 -66 -8 -74 -5 -80 Z" fill="url(#sgHair)"/>' +
      '<path d="' + faceD + '" fill="url(#sgSkin)" stroke="#8a5536" stroke-width=".7"/>' +
      '<g clip-path="url(#sgFaceClip)">' +
        '<rect x="-12" y="-96" width="42" height="22" fill="url(#sgFaceShade)"/>' +
        '<ellipse cx="13" cy="-63" rx="6" ry="3.6" fill="rgba(255,222,190,.24)"/>' +
        '<path d="M-10 -60 Q0 -52 12 -52 L12 -46 L-12 -46 Z" fill="rgba(50,22,8,.28)"/>' +
        '<path d="M22.6 -73.6 Q23.2 -67.6 25.6 -65" stroke="rgba(120,64,34,.4)" stroke-width=".8" fill="none"/>' +
      '</g>' +
      '<ellipse cx="24.4" cy="-63.9" rx="1.2" ry=".7" fill="#5a2f18"/>' +
      '<path d="M10.8 -71.4 Q14.8 -74.4 19 -71.6 Q15 -68.6 10.8 -71.4 Z" fill="#F4EFE7"/>' +
      '<circle cx="15.8" cy="-71.4" r="2" fill="#5f3a1e"/><circle cx="15.9" cy="-71.4" r="1" fill="#170d05"/><circle cx="16.5" cy="-72.1" r=".55" fill="#fff"/>' +
      '<path d="M10.4 -71.4 Q14.6 -75 19.4 -71.6" stroke="#2a190d" stroke-width="1.3" fill="none" stroke-linecap="round"/>' +
      '<path d="M11 -70.4 Q15 -68.2 18.8 -70.6" stroke="rgba(120,70,40,.45)" stroke-width=".6" fill="none"/>' +
      '<path id="sgLid" d="M10.2 -71.4 Q14.8 -75.4 19.6 -71.6 Q15 -67.8 10.2 -71.4 Z" fill="#E4A67E" opacity="0"/>' +
      '<path d="M8.6 -75.8 Q15 -79.6 21.6 -75.6" stroke="#3b2413" stroke-width="2.5" fill="none" stroke-linecap="round"/>' +
      '<path d="M9.6 -76.6 Q15 -79.6 20 -77" stroke="#5d3a1e" stroke-width="1" fill="none" stroke-linecap="round"/>' +
      '<path d="M-8.6 -62 C-8 -52 -2 -46.6 8 -45.6 C18 -46 23.4 -51 24 -58.4 C21.6 -57.4 19 -57.2 16.4 -58 C13 -58.6 9.6 -60 6 -59.4 C1 -58.6 -3.6 -59.6 -8.6 -62 Z" fill="url(#sgBeard)" stroke="#2a180a" stroke-width=".5"/>' +
      '<path d="M-6 -58 C-4 -52 0 -49 6 -48.2 M-2 -57.6 C0 -52 4 -49.4 10 -48.6 M3 -58 C5 -53 9 -50.4 14 -50.4 M9 -58.6 C11 -54 14 -52 18 -52.6 M-4 -55 C-3 -51 0 -48.4 4 -47.4" stroke="rgba(28,15,6,.55)" stroke-width=".7" fill="none" stroke-linecap="round"/>' +
      '<path d="M-3 -57 C-1 -53 2 -51 6 -50.6 M6 -57 C8 -53.6 11 -52 14 -52" stroke="rgba(190,140,90,.4)" stroke-width=".6" fill="none" stroke-linecap="round"/>' +
      '<path d="M11.6 -60.6 C15 -61.8 19.4 -60.6 23.8 -60.8 C21.8 -56.8 18 -56.4 15 -57.4 C13.4 -58 12.4 -58.8 11.6 -60.6 Z" fill="url(#sgBeard)" stroke="#2a180a" stroke-width=".4"/>' +
      '<path d="M16.4 -56.8 Q19.6 -56.2 22 -57.2" stroke="rgba(15,7,2,.7)" stroke-width=".8" fill="none" stroke-linecap="round"/>' +
      '<path d="M-3 -72 C-6.4 -71.6 -6.8 -65 -3.4 -63.4 C-1.4 -62.6 0.4 -64.4 0.2 -67 C0 -70 -1 -71.8 -3 -72 Z" fill="#DBA07A" stroke="#9a6446" stroke-width=".6"/>' +
      '<path d="M-3.2 -69.6 C-4.6 -68.6 -4.4 -66 -2.6 -65.4" stroke="rgba(120,70,44,.65)" fill="none" stroke-width=".7"/>' +
      '<path d="M-15 -79 C-16 -104 4 -108 16 -105 C26 -102 29 -92 29 -82 L-15 -79 Z" fill="url(#sgHat)" stroke="#a87500" stroke-width=".8"/>' +
      '<path d="M-15 -79 C-16 -96 -8 -104 0 -105.4 L-3 -80 Z" fill="rgba(120,70,0,.34)"/>' +
      '<path d="M-6 -106 C4 -109.6 16 -106.8 22 -100 L19 -97 C13 -101 5 -102.6 -4 -100 Z" fill="#FFF3A6" opacity=".85"/>' +
      '<path d="M2 -106.8 L4 -98.6 M9 -106.6 L11 -98.4 M16 -105 L17.6 -97.6" stroke="rgba(255,255,255,.4)" stroke-width="1" stroke-linecap="round"/>' +
      '<path d="M-15 -88.6 C0 -86.6 18 -86.6 29.4 -90.4 L29.4 -85.8 C18 -82.4 0 -82.4 -15 -84.6 Z" fill="#20242a" stroke="#0c0e12" stroke-width=".6"/>' +
      '<path d="M-17 -80 L36 -80.6 Q40.6 -79.6 39.4 -75.6 C34 -75 20 -75.4 -17 -76.6 Z" fill="#D69200" stroke="#8f6000" stroke-width=".7"/>' +
      '<path d="M-17 -76.6 C20 -75.4 34 -75 39.4 -75.6" stroke="rgba(0,0,0,.35)" stroke-width=".8" fill="none"/>' +
      '<rect x="26" y="-96" width="11" height="8.4" rx="2.4" fill="#B3BBC5" stroke="#565e68" stroke-width=".7"/>' +
      '<rect x="27.4" y="-94.8" width="5" height="2" rx="1" fill="rgba(255,255,255,.55)"/>' +
      '<circle cx="37.2" cy="-91.6" r="3.4" fill="url(#sgLens)"/><circle cx="37.2" cy="-91.6" r="8" fill="url(#sgLens)" opacity=".4"/>' +
      '<path d="M37.2 -98 L37.2 -85.2 M30.8 -91.6 L43.6 -91.6" stroke="rgba(255,250,215,.5)" stroke-width=".7"/>';
    var lidEl = headG.querySelector('#sgLid');

    var pickG = el('g', { id: 'sgPick' }, torsoG);
    var wrap = '';
    for (var wi = 26; wi < 60; wi += 3.6) wrap += 'M' + wi + ' -4 L' + (wi + 3) + ' 4 ';
    pickG.innerHTML =
      '<path d="M6 -4.6 Q40 -3.7 106 -3.4 L106 3.4 Q40 3.7 6 4.6 Q3 0 6 -4.6 Z" fill="url(#sgWood2)" stroke="#43231a" stroke-width=".7"/>' +
      '<path d="M10 -1.8 Q40 -2.6 74 -1.2 T106 -1.6 M12 1.6 Q44 2.4 80 1 T106 1.8 M20 .2 Q50 -.6 90 .5 T106 0" stroke="rgba(58,28,20,.34)" stroke-width=".7" fill="none"/>' +
      '<ellipse cx="82" cy="-.6" rx="2.2" ry="1" fill="rgba(58,28,20,.35)"/>' +
      '<path d="M9 -3.2 Q40 -2.8 104 -2.5" stroke="rgba(255,238,224,.5)" stroke-width="1.1" fill="none" stroke-linecap="round"/>' +
      '<rect x="24" y="-4.4" width="38" height="8.8" rx="2" fill="url(#sgLeather)" stroke="#170e08" stroke-width=".7"/>' +
      '<path d="' + wrap + '" stroke="rgba(255,235,205,.16)" stroke-width=".9" fill="none"/>' +
      '<path d="M25 -3.4 L61 -3.4" stroke="rgba(255,255,255,.14)" stroke-width=".8"/>' +
      '<path d="M2 -5.6 L8 -5.6 L8 5.6 L2 5.6 Q0 0 2 -5.6 Z" fill="url(#sgSteel2)" stroke="#3a3448" stroke-width=".6"/>' +
      '<path d="M98 -36 C105 -25 110 -11 110 0 C110 11 105 25 98 36 C101 19 97 10 97 0 C97 -10 101 -19 98 -36 Z" fill="#2a2437" stroke="#241f30" stroke-width="2.2" stroke-linejoin="round"/>' +
      '<path d="M98 -36 C105 -25 110 -11 110 0 C110 11 105 25 98 36 C101 19 97 10 97 0 C97 -10 101 -19 98 -36 Z" fill="url(#sgSteel2)" stroke="#3a3448" stroke-width=".8" stroke-linejoin="round"/>' +
      '<path d="M101 -32 C106 -22 107.6 -11 107.6 0" stroke="url(#sgSteelEdge)" stroke-width="2.6" fill="none" stroke-linecap="round" opacity=".95"/>' +
      '<path d="M101 32 C106 22 107.6 11 107.6 0" stroke="rgba(38,30,60,.6)" stroke-width="2.4" fill="none" stroke-linecap="round"/>' +
      '<path d="M102 -22 C104.6 -15 103 -8 103 0 C103 8 104.6 15 102 22" stroke="rgba(255,255,255,.2)" stroke-width="1" fill="none"/>' +
      '<path d="M102 -27 C106 -19 108.6 -9 109 -2 M102 27 C106 19 108.6 9 109 2" stroke="rgba(60,50,86,.22)" stroke-width=".8" fill="none"/>' +
      '<path d="M103.6 -27 C106.4 -23 108.2 -18 109 -13" stroke="#fff" stroke-width="1.8" fill="none" stroke-linecap="round" opacity=".9"/>' +
      '<circle cx="98.4" cy="-35.4" r="1.3" fill="#fff"/><circle cx="98.4" cy="35.4" r="1.1" fill="#d9d2e8"/>' +
      '<path d="M107 -8 l2.4 1.2 M108 7 l2 -.8 M104 17 l2.2 1.8" stroke="rgba(30,24,44,.4)" stroke-width=".8" stroke-linecap="round"/>' +
      '<path d="M95 -9.6 L111 -10.4 Q113.6 0 111 10.4 L95 9.6 Q92.6 0 95 -9.6 Z" fill="url(#sgSocket)" stroke="#1a1524" stroke-width=".9"/>' +
      '<path d="M97 -7.8 L109 -8.6" stroke="rgba(255,255,255,.4)" stroke-width="1.1" stroke-linecap="round"/>' +
      '<path d="M104 -10 L104 10" stroke="rgba(10,6,20,.6)" stroke-width="1"/>' +
      '<circle cx="100" cy="-5.4" r="1.7" fill="#9a91ae" stroke="#231e30" stroke-width=".8"/><circle cx="100" cy="5.4" r="1.7" fill="#9a91ae" stroke="#231e30" stroke-width=".8"/>' +
      '<circle cx="99.6" cy="-5.8" r=".6" fill="#fff" opacity=".8"/><circle cx="99.6" cy="5" r=".6" fill="#fff" opacity=".8"/>';

    var nearArmG = el('g', {}, torsoG);
    var fUpper = limb(farArmG, '#2A4266', '#12203a', '#5d7cab'), fFore = limb(farArmG, '#B87F5A', '#6e4630', '#dba17a');
    var fCuff = limb(farArmG, '#365683', '#12203a', '#7896c4');
    var fGloveG = glove(farArmG, .86);
    var nUpper = limb(nearArmG, '#3F5F8C', '#1a2c48', '#86a6d2'), nFore = limb(nearArmG, '#D9976E', '#8a5638', '#f6c9a6');
    var nCuff = limb(nearArmG, '#4a6c9a', '#1a2c48', '#93b3de');
    var nGloveG = glove(nearArmG, 1);
    var fShoulder = el('circle', { r: '6.6', fill: '#2A4266', stroke: '#12203a', 'stroke-width': '1' }, farArmG);
    var nShoulder = el('circle', { r: '7', fill: '#3F5F8C', stroke: '#1a2c48', 'stroke-width': '1' }, nearArmG);

    var SN = { x: 4, y: -42 }, SF = { x: -1, y: -44 };
    var tipLocal = [{ x: 98, y: -36 }, { x: 110, y: 0 }, { x: 98, y: 36 }];   // pick head: upper tip, apex, lower tip
    function headPoints() {
      // world-space (scene) positions of the pick head, used for the swing smear
      var a = st.arm * Math.PI / 180, ca = Math.cos(a), sa = Math.sin(a);
      var l = st.lean * Math.PI / 180, cl = Math.cos(l), sl = Math.sin(l);
      var out = [];
      for (var i = 0; i < 3; i++) {
        var px = SN.x + ca * tipLocal[i].x - sa * tipLocal[i].y, py = SN.y + sa * tipLocal[i].x + ca * tipLocal[i].y;
        out.push({ x: 250 + cl * px - sl * py, y: 168 + st.dy + sl * px + cl * py });
      }
      return out;
    }
    function updateMiner() {
      var dy = st.dy, lift = st.lift;
      var hb = { x: -4, y: 2 }, hf = { x: 6, y: 2 };
      var af = { x: 26, y: 50 - dy }, ab = { x: -40, y: 50 - dy - lift * .5 };
      var kb = ik(hb.x, hb.y, ab.x, ab.y, 34, 32, true), kf = ik(hf.x, hf.y, af.x, af.y, 34, 32, true);
      setLimb(bThigh, hb, kb.e, 14); setLimb(bShin, kb.e, kb.h, 10.5);
      bKnee.setAttribute('cx', kb.e.x.toFixed(2)); bKnee.setAttribute('cy', kb.e.y.toFixed(2));
      bKnee.setAttribute('transform', 'rotate(' + (Math.atan2(kb.h.y - kb.e.y, kb.h.x - kb.e.x) * 180 / Math.PI - 90).toFixed(1) + ' ' + kb.e.x.toFixed(2) + ' ' + kb.e.y.toFixed(2) + ')');
      bBoot.setAttribute('transform', 'translate(' + kb.h.x.toFixed(2) + ' ' + (kb.h.y + 1).toFixed(2) + ') rotate(' + (-lift * 1.9).toFixed(2) + ' 16 12)');
      setLimb(fThigh, hf, kf.e, 15); setLimb(fShin, kf.e, kf.h, 11.4);
      var tAng = Math.atan2(kf.e.y - hf.y, kf.e.x - hf.x) * 180 / Math.PI - 90, tp = lerpPt(hf, kf.e, .42);
      fPocket.setAttribute('transform', 'translate(' + (tp.x + 2.4).toFixed(2) + ' ' + tp.y.toFixed(2) + ') rotate(' + tAng.toFixed(1) + ')');
      fPocketLine.setAttribute('transform', 'translate(' + (tp.x + 2.4).toFixed(2) + ' ' + tp.y.toFixed(2) + ') rotate(' + tAng.toFixed(1) + ')');
      var sAng = Math.atan2(kf.h.y - kf.e.y, kf.h.x - kf.e.x) * 180 / Math.PI - 90;
      fKnee.setAttribute('cx', kf.e.x.toFixed(2)); fKnee.setAttribute('cy', kf.e.y.toFixed(2));
      fKnee.setAttribute('transform', 'rotate(' + sAng.toFixed(1) + ' ' + kf.e.x.toFixed(2) + ' ' + kf.e.y.toFixed(2) + ')');
      var bend = Math.abs(sAng - tAng), wr = Math.min(1, bend / 60), kx = kf.e.x - 5.4, ky = kf.e.y - 1;
      fWrink.setAttribute('d', 'M' + kx.toFixed(1) + ' ' + (ky + 3).toFixed(1) + ' q3 ' + (-1.6 * wr - .4).toFixed(1) + ' 6 0 M' + (kx + .4).toFixed(1) + ' ' + (ky + 6.4).toFixed(1) + ' q3 ' + (-1.4 * wr - .3).toFixed(1) + ' 5.4 0 M' + (kx + .6).toFixed(1) + ' ' + (ky - 2.2).toFixed(1) + ' q3 ' + (-1.2 * wr - .3).toFixed(1) + ' 5 0');
      fBoot.setAttribute('transform', 'translate(' + kf.h.x.toFixed(2) + ' ' + (kf.h.y + 1).toFixed(2) + ')');

      var a = st.arm * Math.PI / 180, c = Math.cos(a), s = Math.sin(a);
      var handN = { x: SN.x + c * 52, y: SN.y + s * 52 }, handR = { x: SN.x + c * 30, y: SN.y + s * 30 };
      var an = ik(SN.x, SN.y, handN.x, handN.y, 30, 28, false), af2 = ik(SF.x, SF.y, handR.x, handR.y, 30, 28, false);
      setLimb(nUpper, SN, an.e, 11.6); setLimb(nFore, an.e, an.h, 8.8);
      setLimb(nCuff, an.e, lerpPt(an.e, an.h, .3), 12.4);
      nGloveG.setAttribute('transform', 'translate(' + an.h.x.toFixed(2) + ' ' + an.h.y.toFixed(2) + ') rotate(' + st.arm.toFixed(1) + ')');
      nShoulder.setAttribute('cx', SN.x); nShoulder.setAttribute('cy', SN.y);
      setLimb(fUpper, SF, af2.e, 10.6); setLimb(fFore, af2.e, af2.h, 8);
      setLimb(fCuff, af2.e, lerpPt(af2.e, af2.h, .3), 11.4);
      fGloveG.setAttribute('transform', 'translate(' + af2.h.x.toFixed(2) + ' ' + af2.h.y.toFixed(2) + ') rotate(' + st.arm.toFixed(1) + ')');
      fShoulder.setAttribute('cx', SF.x); fShoulder.setAttribute('cy', SF.y);
      pickG.setAttribute('transform', 'translate(' + SN.x + ' ' + SN.y + ') rotate(' + st.arm.toFixed(2) + ')');
      // proportions: a realistic head (about 1/6 of body height), steadied against the torso lean
      headG.setAttribute('transform', 'translate(2 -49) rotate(' + (-st.lean * .35).toFixed(2) + ') scale(.6) translate(-2 49)');
    }

    /* coin sprite: pre-rendered once so many coins can be drawn cheaply */
    var SPR = 128, sprite = document.createElement('canvas'), spriteReady = false;
    sprite.width = sprite.height = SPR;
    var coinImg = new Image();
    coinImg.onload = function () {
      var k = SPR / (coinImg.naturalWidth * 0.806);   // the coin fills ~81% of the artwork
      var w = coinImg.naturalWidth * k, h = coinImg.naturalHeight * k;
      sprite.getContext('2d').drawImage(coinImg, (SPR - w) / 2, (SPR - h) / 2, w, h);
      spriteReady = true;
    };
    coinImg.src = COIN_URL;

    var W = 0, H = 0, K = 1, DPR = 1, VL = 0, VR = VBW;
    function resize() {
      var r = scene.getBoundingClientRect();
      W = r.width; H = r.height; K = H / 272;
      DPR = Math.min(window.devicePixelRatio || 1, 2);
      cv.width = Math.max(1, Math.round(W * DPR));
      cv.height = Math.max(1, Math.round(H * DPR));
      VL = 385 - W / (2 * K); VR = 385 + W / (2 * K);   // visible range, in scene units
    }

    var coins = [], sparks = [], chips = [], dust = [], trail = [];
    var st = { active: false, t: 0, prev: 0, strikes: 0, progress: 0, arm: -20, lean: -4, dy: 0, lift: 0, clock: 0, nextBlink: 2.2, shake: 0, glow: 0, raf: 0, last: 0 };

    /* ---- swing timeline: wind-up, fast strike, recoil, recover ---- */
    function lerp(a, b, t) { return a + (b - a) * t; }
    function easeInOut(t) { return t < .5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2; }
    function easeIn(t) { return t * t * t; }
    function pose(ph) {
      var u;
      if (ph < .46) { u = easeInOut(ph / .46); return { arm: lerp(-20, -146, u), lean: lerp(-4, -12, u), dy: lerp(0, -3, u), lift: 0 }; }
      if (ph < IMP) { u = easeIn((ph - .46) / (IMP - .46)); return { arm: lerp(-146, -26, u), lean: lerp(-12, 14, u), dy: lerp(-3, 5, u), lift: lerp(0, 9, u) }; }
      if (ph < .66) { u = (ph - IMP) / (.66 - IMP); return { arm: -26 - 14 * Math.sin(u * Math.PI) * Math.exp(-1.6 * u), lean: lerp(14, 10, u), dy: lerp(5, 2, u), lift: lerp(9, 4, u) }; }
      u = easeInOut((ph - .66) / .34);
      return { arm: lerp(-26, -20, u), lean: lerp(10, -4, u), dy: lerp(2, 0, u), lift: lerp(4, 0, u) };
    }
    function applyPose(ts) {
      minerG.setAttribute('transform', 'translate(250 ' + (168 + st.dy).toFixed(2) + ')');
      torsoG.setAttribute('transform', 'rotate(' + st.lean.toFixed(2) + ')');
      updateMiner();
      bodyG.setAttribute('transform', 'scale(1 ' + (1 + .007 * Math.sin(st.clock * 2.3)).toFixed(4) + ')');
      lidEl.setAttribute('opacity', (st.clock > st.nextBlink && st.clock < st.nextBlink + .12) ? '1' : '0');
      if (st.clock > st.nextBlink + .12) st.nextBlink = st.clock + 2.4 + Math.random() * 2.6;
      shadowEl.setAttribute('cx', (250 + st.lean * .9).toFixed(1));
      var s = st.shake;
      rockG.setAttribute('transform', s > .01 ? 'translate(' + (Math.sin(ts * .09) * 3 * s).toFixed(2) + ' ' + (Math.cos(ts * .13) * 1.6 * s).toFixed(2) + ')' : '');
      glowEl.setAttribute('opacity', Math.min(1, st.glow).toFixed(2));
    }

    /* ---- effects ---- */
    function coinCount() {
      if (st.strikes < 3) return 0;                     // the first strikes only chip the rock
      var p = st.progress, n = 1 + (Math.random() < .55 ? 1 : 0) + (p > 20 ? 1 : 0) + (p > 45 ? 1 : 0) + (p > 70 ? 1 : 0);
      return Math.min(n, 5);
    }
    function spawnCoin() {
      var ang = -Math.PI / 2 + (Math.random() - .42) * 2.2;     // fan upward, a little biased away from the miner
      var sp = 380 + Math.random() * 260;
      var r = 15 + Math.random() * 6;
      coins.push({
        x: SOURCE.x, y: SOURCE.y, vx: Math.cos(ang) * sp, vy: Math.sin(ang) * sp, r: r,
        gy: GY + Math.random() * 12 - 3, spin: Math.random() * 6.28, spinV: (8 + Math.random() * 12) * (Math.random() < .5 ? -1 : 1),
        a: 1, age: 0, rest: false, life: 7 + Math.random() * 4
      });
      if (coins.length > 26) { for (var i = 0; i < coins.length; i++) { if (coins[i].rest) { coins[i].life = Math.min(coins[i].life, 0); break; } } }
    }
    function impact() {
      st.strikes++; st.shake = 1; st.glow = 1;
      var ix = IMPACT.x, iy = IMPACT.y, i, a, sp;
      for (i = 0; i < 16; i++) {
        a = -Math.PI * .5 - (Math.random() * 1.9 - .4); sp = 240 + Math.random() * 320;
        sparks.push({ x: ix, y: iy, vx: Math.cos(a) * sp, vy: Math.sin(a) * sp, life: .28 + Math.random() * .3, age: 0 });
      }
      for (i = 0; i < 7; i++) {
        a = -Math.PI / 2 + (Math.random() - .5) * 2.6; sp = 170 + Math.random() * 230;
        chips.push({ x: ix + 4, y: iy + 4, vx: Math.cos(a) * sp, vy: Math.sin(a) * sp, s: 2.5 + Math.random() * 3.5, rot: Math.random() * 6, rv: (Math.random() - .5) * 16, bounced: false, a: 1 });
      }
      for (i = 0; i < 5; i++) {
        dust.push({ x: ix + Math.random() * 14 - 4, y: iy + 10 + Math.random() * 14, vx: -30 - Math.random() * 60, vy: -20 - Math.random() * 40, r: 6 + Math.random() * 6, a: .9 });
      }
      var n = coinCount();
      for (i = 0; i < n; i++) spawnCoin();
    }

    function physics(dt) {
      var i, j, c, p;
      for (i = coins.length - 1; i >= 0; i--) {
        c = coins[i]; c.age += dt;
        c.vy += 1750 * dt; c.x += c.vx * dt; c.y += c.vy * dt; c.spin += c.spinV * dt;
        var floor = c.gy - c.r * .92;
        if (c.y > floor) {
          c.y = floor;
          if (c.vy > 110) { c.vy *= -.46; c.vx *= .7; c.spinV *= .72; } else { c.vy = 0; c.rest = true; }
        }
        if (c.rest) {
          c.vx *= Math.pow(.02, dt); c.spinV *= Math.pow(.03, dt);
          var target = Math.round(c.spin / Math.PI) * Math.PI;
          c.spin += (target - c.spin) * Math.min(1, dt * 5);
          if (Math.abs(c.vx) < 4) c.vx = 0;
          c.life -= dt;
          if (c.life < 0) c.a -= dt * .9;
        }
        var wl = Math.max(120, VL + c.r + 6), wr = Math.min(650, VR - c.r - 6);
        if (c.x < wl) { c.x = wl; c.vx = Math.abs(c.vx) * .5; } else if (c.x > wr) { c.x = wr; c.vx = -Math.abs(c.vx) * .5; }
        if (c.a <= 0) coins.splice(i, 1);
      }
      for (i = 0; i < coins.length; i++) {
        for (j = i + 1; j < coins.length; j++) {
          var A = coins[i], B = coins[j], dx = B.x - A.x, dy = B.y - A.y, d = Math.sqrt(dx * dx + dy * dy), min = (A.r + B.r) * .78;
          if (d < min && d > .001) {
            var nx = dx / d, ny = dy / d, ov = (min - d) / 2;
            A.x -= nx * ov; A.y -= ny * ov; B.x += nx * ov; B.y += ny * ov;
            var rv = (B.vx - A.vx) * nx + (B.vy - A.vy) * ny;
            if (rv < 0) {
              var imp = -rv * .55;
              A.vx -= imp * nx; A.vy -= imp * ny; B.vx += imp * nx; B.vy += imp * ny;
              if (Math.abs(A.vy) > 45) A.rest = false;
              if (Math.abs(B.vy) > 45) B.rest = false;
            }
          }
        }
      }
      for (i = sparks.length - 1; i >= 0; i--) {
        p = sparks[i]; p.age += dt; p.vy += 900 * dt; p.x += p.vx * dt; p.y += p.vy * dt;
        if (p.age > p.life) sparks.splice(i, 1);
      }
      for (i = chips.length - 1; i >= 0; i--) {
        p = chips[i]; p.vy += 1500 * dt; p.x += p.vx * dt; p.y += p.vy * dt; p.rot += p.rv * dt;
        if (p.y > GY - 2) { p.y = GY - 2; if (!p.bounced) { p.vy *= -.35; p.vx *= .6; p.bounced = true; } else { p.vy = 0; p.vx = 0; p.rv = 0; p.a -= dt * 1.2; } }
        if (p.a <= 0) chips.splice(i, 1);
      }
      for (i = trail.length - 1; i >= 0; i--) { trail[i].age += dt; if (trail[i].age > .18) trail.splice(i, 1); }
      for (i = dust.length - 1; i >= 0; i--) {
        p = dust[i]; p.x += p.vx * dt; p.y += p.vy * dt; p.r += dt * 26; p.a -= dt * 1.1;
        if (p.a <= 0) dust.splice(i, 1);
      }
    }

    function draw() {
      ctx.setTransform(DPR, 0, 0, DPR, 0, 0);
      ctx.clearRect(0, 0, W, H);
      ctx.setTransform(DPR * K, 0, 0, DPR * K, DPR * (W / 2 - 385 * K), DPR * 12 * K);
      var i, c, p;
      for (i = 0; i < dust.length; i++) {
        p = dust[i]; ctx.globalAlpha = Math.max(0, p.a) * .32; ctx.fillStyle = '#9aa4b1';
        ctx.beginPath(); ctx.arc(p.x, p.y, p.r, 0, 6.283); ctx.fill();
      }
      ctx.fillStyle = '#000';
      for (i = 0; i < coins.length; i++) {
        c = coins[i];
        var h = Math.max(0, (c.gy - c.r * .92) - c.y), s = 1 / (1 + h / 110);
        ctx.globalAlpha = c.a * .42 * s;
        ctx.beginPath(); ctx.ellipse(c.x, c.gy + 2, c.r * (.95 * s + .1), c.r * .2 * s + 1, 0, 0, 6.283); ctx.fill();
      }
      for (i = 1; i < trail.length; i++) {
        var TA = trail[i - 1], TB = trail[i];
        ctx.globalAlpha = Math.max(0, 1 - TB.age / .18) * .34; ctx.fillStyle = '#E4DEF6';
        ctx.beginPath(); ctx.moveTo(TA.p[0].x, TA.p[0].y); ctx.lineTo(TB.p[0].x, TB.p[0].y); ctx.lineTo(TB.p[2].x, TB.p[2].y); ctx.lineTo(TA.p[2].x, TA.p[2].y); ctx.closePath(); ctx.fill();
      }
      ctx.fillStyle = '#4a505b';
      for (i = 0; i < chips.length; i++) {
        p = chips[i]; ctx.globalAlpha = Math.max(0, p.a);
        ctx.save(); ctx.translate(p.x, p.y); ctx.rotate(p.rot);
        ctx.beginPath(); ctx.moveTo(-p.s, -p.s * .6); ctx.lineTo(p.s, -p.s * .3); ctx.lineTo(p.s * .5, p.s); ctx.lineTo(-p.s * .8, p.s * .5); ctx.closePath(); ctx.fill();
        ctx.restore();
      }
      for (i = 0; i < coins.length; i++) {
        c = coins[i];
        var cs = Math.cos(c.spin), w = Math.abs(cs);
        ctx.save(); ctx.globalAlpha = c.a; ctx.translate(c.x, c.y);
        ctx.scale((cs < 0 ? -1 : 1) * Math.max(w, .16), 1);
        if (spriteReady) {
          ctx.drawImage(sprite, -c.r, -c.r, c.r * 2, c.r * 2);
        } else {
          ctx.fillStyle = '#E2A93B'; ctx.beginPath(); ctx.arc(0, 0, c.r, 0, 6.283); ctx.fill();
          ctx.strokeStyle = '#8a5a12'; ctx.lineWidth = 2; ctx.stroke();
        }
        if (w < .6) { ctx.fillStyle = 'rgba(70,40,5,' + ((.6 - w) * .9).toFixed(2) + ')'; ctx.beginPath(); ctx.arc(0, 0, c.r, 0, 6.283); ctx.fill(); }
        ctx.restore();
      }
      ctx.save(); ctx.globalCompositeOperation = 'lighter'; ctx.lineCap = 'round';
      for (i = 0; i < sparks.length; i++) {
        p = sparks[i]; var k = 1 - p.age / p.life;
        ctx.globalAlpha = Math.max(0, k); ctx.strokeStyle = k > .55 ? '#FFF1B8' : '#FFAE3A'; ctx.lineWidth = 2;
        ctx.beginPath(); ctx.moveTo(p.x, p.y); ctx.lineTo(p.x - p.vx * .022, p.y - p.vy * .022); ctx.stroke();
      }
      ctx.restore();
      ctx.globalAlpha = 1;
    }

    function tick(ts) {
      st.raf = 0;
      var dt = st.last ? Math.min(.05, (ts - st.last) / 1000) : .016;
      st.last = ts;
      if (st.active) {
        st.t += dt;
        var ph = (st.t % CYCLE) / CYCLE;
        if (st.prev < IMP && ph >= IMP) impact();
        st.prev = ph;
        var p = pose(ph);
        st.arm = p.arm; st.lean = p.lean; st.dy = p.dy; st.lift = p.lift;
      } else {
        var e = Math.min(1, dt * 6);
        st.arm += (-20 - st.arm) * e; st.lean += (-4 - st.lean) * e; st.dy += (0 - st.dy) * e; st.lift += (0 - st.lift) * e;
      }
      st.clock += dt;
      if (st.active && st.prev > .38 && st.prev < .62) trail.push({ p: headPoints(), age: 0 });
      st.shake *= Math.pow(.001, dt);
      st.glow *= Math.pow(.02, dt);
      applyPose(ts);
      physics(dt);
      draw();
      if (st.active || coins.length || sparks.length || chips.length || dust.length || trail.length || Math.abs(st.arm + 20) > .3 || st.glow > .01 || st.shake > .01) {
        st.raf = track.raf(tick);
      } else {
        st.last = 0;
      }
    }
    function setActive(on) {
      if (reduce) { applyPose(0); return; }
      if (on && !st.active) { st.t = 0; st.prev = 0; st.strikes = 0; }
      st.active = on;
      if (!st.raf) st.raf = track.raf(tick);
    }

    function syncStatus() {
      var s = statusEl ? (statusEl.textContent || '').trim().toLowerCase() : '';
      var on = s === 'active';
      scene.classList.toggle('is-active', on);
      labelText.textContent = on ? 'MINING' : (s && s !== 'loading...' ? 'IDLE' : 'STANDBY');
      setActive(on);
    }
    function syncProgress() {
      var w = barEl ? parseFloat(barEl.style.width) : 0;
      st.progress = isFinite(w) ? Math.max(0, Math.min(100, w)) : 0;
      gauge.style.setProperty('--p', st.progress);
    }

    resize();
    if (window.ResizeObserver) track.observer(new ResizeObserver(resize)).observe(scene); else track.on(window, 'resize', resize);
    applyPose(0);
    syncProgress();
    syncStatus();
    if (window.MutationObserver) {
      if (statusEl) track.observer(new MutationObserver(syncStatus)).observe(statusEl, { childList: true, characterData: true, subtree: true });
      if (barEl) track.observer(new MutationObserver(syncProgress)).observe(barEl, { attributes: true, attributeFilter: ['style'] });
      track.observer(new MutationObserver(function () {
        tokenEl.classList.remove('sg-tick');
        void tokenEl.offsetWidth;
        tokenEl.classList.add('sg-tick');
      })).observe(tokenEl, { childList: true, characterData: true, subtree: true });
    }
  })();

  if (reduce) return cleanup;

  /* HUD frame */
  var hud = document.createElement('div');
  hud.className = 'sg-hud';
  hud.setAttribute('aria-hidden', 'true');
  hud.innerHTML = '<i></i><i></i><i></i><i></i>';
  document.body.appendChild(track.node(hud));

  /* cursor spotlight on cards */
  if (content && fine) {
    var raf = 0, last = null;
    track.on(content, 'mousemove', function (e) {
      last = e;
      if (raf) return;
      raf = track.raf(function () {
        raf = 0;
        var card = last.target.closest && last.target.closest('.card');
        if (!card) return;
        var r = card.getBoundingClientRect();
        card.style.setProperty('--mx', (last.clientX - r.left) + 'px');
        card.style.setProperty('--my', (last.clientY - r.top) + 'px');
      });
    });
    track.on(content, 'mouseout', function (e) {
      var card = e.target.closest && e.target.closest('.card');
      if (card && !card.contains(e.relatedTarget)) {
        card.style.setProperty('--mx', '-999px');
        card.style.setProperty('--my', '-999px');
      }
    });
  }

  // Keep financial values as rendered from the API; do not replace them
  // with intermediate count-up values.

  return cleanup;
}
