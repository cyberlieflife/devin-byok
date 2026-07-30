/**
 * Devin BYOK — 会话栏上下文圆圈悬停：上方弹出四色挖空环 + 明细
 * 目标节点：.chat-context-usage-widget
 */
(function () {
  if (window.__byokCtxUsage) return;
  window.__byokCtxUsage = true;

  var POP_ID = "byok-ctx-usage-pop";
  var STYLE_ID = "byok-ctx-usage-style";
  var COLORS = {
    system: "#94a3b8",
    tools: "#a78bfa",
    conversation: "#f59e0b",
    other: "#64748b",
    free: "rgba(255,255,255,0.08)"
  };

  function ensureStyle() {
    if (document.getElementById(STYLE_ID)) return;
    var s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = [
      "#" + POP_ID + "{position:fixed;z-index:100000;min-width:220px;max-width:280px;",
      "background:rgba(30,30,34,0.96);border:1px solid rgba(255,255,255,0.1);",
      "border-radius:10px;padding:12px 14px;box-shadow:0 8px 28px rgba(0,0,0,0.45);",
      "color:#e8e8ed;font:12px/1.45 system-ui,-apple-system,sans-serif;",
      "pointer-events:none;opacity:0;transform:translate(-50%,-8px);",
      "transition:opacity .12s ease;backdrop-filter:blur(8px)}",
      "#" + POP_ID + ".show{opacity:1}",
      "#" + POP_ID + " .hd{display:flex;justify-content:space-between;align-items:baseline;margin-bottom:8px}",
      "#" + POP_ID + " .hd .t{font-weight:600;font-size:13px}",
      "#" + POP_ID + " .hd .p{color:#9ca3af;font-variant-numeric:tabular-nums}",
      "#" + POP_ID + " .donut-wrap{position:relative;width:112px;height:112px;margin:6px auto 10px}",
      "#" + POP_ID + " .donut{width:112px;height:112px;border-radius:50%;",
      "background:conic-gradient(#64748b 0deg, #64748b 360deg);",
      "-webkit-mask:radial-gradient(farthest-side,transparent 56%,#000 57%);",
      "mask:radial-gradient(farthest-side,transparent 56%,#000 57%)}",
      "#" + POP_ID + " .donut-center{position:absolute;inset:0;display:flex;flex-direction:column;",
      "align-items:center;justify-content:center;pointer-events:none}",
      "#" + POP_ID + " .donut-center .pct{font-size:18px;font-weight:600;letter-spacing:-0.02em}",
      "#" + POP_ID + " .donut-center .sub{font-size:10px;color:#9ca3af;margin-top:2px}",
      "#" + POP_ID + " .tok{text-align:center;color:#9ca3af;font-size:11px;margin-bottom:8px;",
      "font-variant-numeric:tabular-nums}",
      "#" + POP_ID + " .leg{display:flex;flex-direction:column;gap:5px}",
      "#" + POP_ID + " .leg-row{display:flex;align-items:center;gap:8px}",
      "#" + POP_ID + " .sw{width:8px;height:8px;border-radius:2px;flex-shrink:0}",
      "#" + POP_ID + " .leg-lab{flex:1;color:#d1d5db}",
      "#" + POP_ID + " .leg-val{color:#9ca3af;font-variant-numeric:tabular-nums}",
      /* 隐藏原生 Context Usage 悬停面板，改用我们的 */
      ".workbench-hover:has(.chat-context-usage-details),",
      ".monaco-hover:has(.chat-context-usage-details){opacity:0!important;pointer-events:none!important;}"
    ].join("");
    document.documentElement.appendChild(s);
  }

  function ensurePop() {
    var el = document.getElementById(POP_ID);
    if (el) return el;
    el = document.createElement("div");
    el.id = POP_ID;
    el.innerHTML = [
      '<div class="hd"><span class="t">Context Usage</span><span class="p" data-p></span></div>',
      '<div class="donut-wrap"><div class="donut" data-donut></div>',
      '<div class="donut-center"><div class="pct" data-pct>0%</div><div class="sub">Full</div></div></div>',
      '<div class="tok" data-tok></div>',
      '<div class="leg" data-leg></div>'
    ].join("");
    document.documentElement.appendChild(el);
    return el;
  }

  function colorForLabel(label) {
    var s = String(label || "").toLowerCase();
    if (s.indexOf("system") >= 0 || s.indexOf("系统") >= 0) return COLORS.system;
    if (s.indexOf("tool") >= 0 || s.indexOf("工具") >= 0) return COLORS.tools;
    if (s.indexOf("conversation") >= 0 || s.indexOf("对话") >= 0 || s.indexOf("chat") >= 0) return COLORS.conversation;
    return COLORS.other;
  }

  function readRingPct(widget) {
    var arc = widget.querySelector("circle.progress-arc");
    if (!arc) return null;
    var c = parseFloat(arc.getAttribute("stroke-dasharray") || "0");
    var o = parseFloat(arc.getAttribute("stroke-dashoffset") || "0");
    if (!(c > 0)) {
      // fallback computed style length
      var len = 2 * Math.PI * 14; // RADIUS=14 from source
      c = len;
      o = parseFloat(arc.style.strokeDashoffset || arc.getAttribute("stroke-dashoffset") || String(len));
    }
    // progress drawn: (c-o)/c
    var used = Math.max(0, Math.min(1, (c - o) / c));
    return used * 100;
  }

  function scrapeNativeDetails() {
    var d = document.querySelector(".chat-context-usage-details");
    if (!d) return null;
    var tokenLabel = (d.querySelector(".token-count-label") || {}).textContent || "";
    var pctRaw = (d.querySelector(".quota-item-value") || {}).textContent || "";
    var pctM = /([\d.]+)\s*%/.exec(pctRaw);
    var pct = pctM ? parseFloat(pctM[1]) : null;
    var segs = [];
    d.querySelectorAll(".token-detail-item").forEach(function (item) {
      var lab = ((item.querySelector(".token-detail-label") || {}).textContent || "").trim();
      var val = ((item.querySelector(".token-detail-value") || {}).textContent || "").trim();
      var m = /([\d.]+)\s*%/.exec(val);
      if (!lab || !m) return;
      segs.push({ label: lab, pct: parseFloat(m[1]), color: colorForLabel(lab) });
    });
    // group headers as fallback labels
    if (!segs.length) {
      d.querySelectorAll(".token-category").forEach(function (cat) {
        var h = ((cat.querySelector(".token-category-header") || {}).textContent || "").trim();
        var sum = 0;
        cat.querySelectorAll(".token-detail-value").forEach(function (v) {
          var m = /([\d.]+)\s*%/.exec(v.textContent || "");
          if (m) sum += parseFloat(m[1]);
        });
        if (h && sum > 0) segs.push({ label: h, pct: sum, color: colorForLabel(h) });
      });
    }
    return { tokenLabel: tokenLabel.trim(), pct: pct, segs: segs };
  }

  function buildConic(segs, usedPct) {
    // segs percentages are of total context window already (from native render)
    var parts = [];
    var angle = 0;
    var acc = 0;
    (segs || []).forEach(function (s) {
      var a = Math.max(0, s.pct) * 3.6;
      if (a <= 0) return;
      parts.push(s.color + " " + angle + "deg " + (angle + a) + "deg");
      angle += a;
      acc += s.pct;
    });
    var rest = Math.max(0, 100 - acc);
    // if no segs, use used vs free
    if (!parts.length && usedPct != null) {
      var u = Math.max(0, Math.min(100, usedPct));
      parts.push(COLORS.conversation + " 0deg " + u * 3.6 + "deg");
      parts.push(COLORS.free + " " + u * 3.6 + "deg 360deg");
      return parts.join(", ");
    }
    if (rest > 0.05) {
      parts.push(COLORS.free + " " + angle + "deg 360deg");
    } else if (angle < 360 && parts.length) {
      // snap last
      parts.push(COLORS.free + " " + angle + "deg 360deg");
    }
    return parts.join(", ") || (COLORS.free + " 0deg 360deg");
  }

  function renderPopup(data, ringPct) {
    var pop = ensurePop();
    var pct = data && data.pct != null ? data.pct : ringPct;
    if (pct == null) pct = 0;
    pop.querySelector("[data-p]").textContent = Math.round(pct) + "% Full";
    pop.querySelector("[data-pct]").textContent = Math.round(pct) + "%";
    pop.querySelector("[data-tok]").textContent = (data && data.tokenLabel) || "";
    var donut = pop.querySelector("[data-donut]");
    donut.style.background = "conic-gradient(" + buildConic(data && data.segs, pct) + ")";
    var leg = pop.querySelector("[data-leg]");
    leg.innerHTML = "";
    var segs = (data && data.segs && data.segs.length) ? data.segs : [
      { label: "Used", pct: pct, color: COLORS.conversation },
      { label: "Free", pct: Math.max(0, 100 - pct), color: COLORS.free }
    ];
    segs.forEach(function (s) {
      if (s.pct < 0.05 && s.label === "Free") return;
      var row = document.createElement("div");
      row.className = "leg-row";
      row.innerHTML = '<span class="sw" style="background:' + s.color + '"></span>' +
        '<span class="leg-lab"></span><span class="leg-val"></span>';
      row.querySelector(".leg-lab").textContent = s.label;
      row.querySelector(".leg-val").textContent = (Math.round(s.pct * 10) / 10) + "%";
      leg.appendChild(row);
    });
    return pop;
  }

  function placeAbove(widget, pop) {
    var r = widget.getBoundingClientRect();
    var x = r.left + r.width / 2;
    var y = r.top;
    pop.style.left = x + "px";
    pop.style.top = y + "px";
    // keep on screen
    requestAnimationFrame(function () {
      var pr = pop.getBoundingClientRect();
      var dy = y - pr.height - 10;
      if (dy < 8) dy = r.bottom + 10;
      var dx = x;
      if (pr.left < 8) dx += 8 - pr.left;
      if (pr.right > window.innerWidth - 8) dx -= pr.right - (window.innerWidth - 8);
      pop.style.left = dx + "px";
      pop.style.top = (dy + pr.height) + "px"; // top is transform origin with translate -100%
      pop.style.transform = "translate(-50%, -100%)";
    });
  }

  var hideTimer = null;
  var scrapeTimer = null;
  var activeWidget = null;

  function hidePop() {
    var pop = document.getElementById(POP_ID);
    if (pop) pop.classList.remove("show");
    activeWidget = null;
  }

  function showFor(widget) {
    ensureStyle();
    activeWidget = widget;
    var ringPct = readRingPct(widget);
    var data = scrapeNativeDetails();
    var pop = renderPopup(data, ringPct);
    placeAbove(widget, pop);
    pop.classList.add("show");

    // native hover 稍后会挂上 details，再刮一次补齐四色
    var tries = 0;
    clearInterval(scrapeTimer);
    scrapeTimer = setInterval(function () {
      tries++;
      var d2 = scrapeNativeDetails();
      if (d2 && d2.segs && d2.segs.length) {
        renderPopup(d2, d2.pct != null ? d2.pct : ringPct);
        placeAbove(widget, pop);
        clearInterval(scrapeTimer);
      } else if (tries > 20) {
        clearInterval(scrapeTimer);
      }
    }, 50);
  }

  function bind(widget) {
    if (widget.dataset.byokCtxBound) return;
    widget.dataset.byokCtxBound = "1";
    widget.addEventListener("mouseenter", function () {
      clearTimeout(hideTimer);
      showFor(widget);
    });
    widget.addEventListener("mouseleave", function () {
      clearTimeout(hideTimer);
      hideTimer = setTimeout(function () {
        hidePop();
        clearInterval(scrapeTimer);
      }, 120);
    });
  }

  function scan() {
    document.querySelectorAll(".chat-context-usage-widget").forEach(bind);
  }

  ensureStyle();
  scan();
  var mo = new MutationObserver(function () { scan(); });
  mo.observe(document.documentElement, { childList: true, subtree: true });
  window.addEventListener("scroll", function () {
    if (activeWidget && document.getElementById(POP_ID)) {
      placeAbove(activeWidget, document.getElementById(POP_ID));
    }
  }, true);
})();
