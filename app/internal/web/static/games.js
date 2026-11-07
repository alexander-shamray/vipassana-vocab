// Spoken games: the site says a prompt, the browser recognises the answer,
// the server grades it. Without speech recognition the player types.
"use strict";

(() => {
  const root = document.getElementById("speech");
  if (!root) return;

  const items = JSON.parse(root.dataset.items || "[]");
  const mode = root.dataset.mode;
  const answerLang = root.dataset.answerLang;
  const Recognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  const $ = (id) => document.getElementById(id);
  const show = (id, on) => { $(id).hidden = !on; };

  let index = -1;
  let current = null; // the running recognition, if any
  const wrong = [];

  if (!Recognition) show("no-mic", true);

  function speakText(item) {
    return new Promise((done) => {
      if (!("speechSynthesis" in window)) return done();
      const u = new SpeechSynthesisUtterance(item.say);
      u.lang = item.lang;
      u.onend = done;
      u.onerror = done;
      speechSynthesis.cancel();
      speechSynthesis.speak(u);
    });
  }

  // Some browsers never fire "end" (no voices, blocked audio): give up
  // waiting after a few seconds rather than freezing the game.
  const atMost = (ms, p) => Promise.race([p, new Promise((done) => setTimeout(done, ms))]);

  function sayPrompt(item) {
    if (!item.audio) return atMost(6000, speakText(item));
    return atMost(6000, new Promise((done) => {
      const a = new Audio(item.audio);
      a.onended = done;
      a.onerror = () => speakText(item).then(done);
      a.play().catch(() => speakText(item).then(done));
    }));
  }

  // Resolves with the recognised alternatives, [] for silence, or null when
  // cancelled by "repeat".
  function listen() {
    return new Promise((done) => {
      const r = new Recognition();
      r.lang = answerLang;
      r.maxAlternatives = 5;
      r.interimResults = false;
      let heard = [];
      r.onresult = (e) => { heard = Array.from(e.results[0], (alt) => alt.transcript); };
      r.onend = () => { if (current === r) { current = null; done(heard); } else done(null); };
      current = r;
      $("status").textContent = "🎤 Говорите…";
      r.start();
    });
  }

  function cancelListening() {
    if (!current) return;
    const r = current;
    current = null; // its onend now reports "cancelled"
    r.abort();
  }

  async function check(item, answers) {
    const body = new URLSearchParams({ key: item.key, mode });
    answers.forEach((a) => body.append("answer", a));
    const resp = await fetch("/games/check", { method: "POST", body });
    if (!resp.ok) throw new Error(`check failed: ${resp.status}`);
    return resp.json();
  }

  async function ask() {
    const item = items[index];
    $("progress").textContent = `${index + 1} / ${items.length}`;
    $("result").textContent = "";
    $("result").className = "result";
    show("next", false);
    show("repeat", true);
    show("skip", true);
    $("status").textContent = "🔊 Слушайте…";
    await sayPrompt(item);
    if (!Recognition) {
      $("status").textContent = "Введите ответ";
      show("typed", true);
      $("typed-input").value = "";
      $("typed-input").focus();
      return;
    }
    const answers = await listen();
    if (answers === null) return; // repeat started a new round
    await grade(item, answers);
  }

  async function grade(item, answers) {
    show("repeat", false);
    show("skip", false);
    show("typed", false);
    $("status").textContent = "";
    let res;
    try {
      res = await check(item, answers);
    } catch (e) {
      $("result").textContent = "Не удалось проверить ответ — попробуйте ещё раз.";
      show("repeat", true);
      return;
    }
    const said = answers.length ? `Вы сказали «${answers[0]}».` : "Нет ответа.";
    if (res.ok) {
      $("result").textContent = `✓ Верно: ${res.lemma} — ${res.expected}`;
      $("result").className = "result ok";
    } else {
      $("result").textContent = `✗ ${said} Правильно: ${res.lemma} — ${res.expected}`;
      $("result").className = "result bad";
      wrong.push({ key: item.key, lemma: res.lemma, expected: res.expected });
    }
    show("next", true);
    $("next").focus();
  }

  function next() {
    index += 1;
    if (index < items.length) return ask();
    finish();
  }

  function finish() {
    ["start", "repeat", "skip", "next", "typed"].forEach((id) => show(id, false));
    $("status").textContent = "";
    $("result").textContent = "";
    $("progress").textContent = "";
    $("score").textContent = `Верно ${items.length - wrong.length} из ${items.length}`;
    show("wrong-hint", wrong.length > 0);
    const list = $("wrong");
    list.replaceChildren();
    for (const w of wrong) {
      const li = document.createElement("li");
      li.className = "bad";
      const form = document.createElement("form");
      form.className = "act";
      form.method = "post";
      const path = `/games/important/${encodeURIComponent(w.key)}`;
      form.action = path;
      form.setAttribute("hx-post", path);
      form.setAttribute("hx-swap", "outerHTML");
      const btn = document.createElement("button");
      btn.type = "submit";
      btn.className = "wrong";
      btn.title = "Отправить в «Важные»";
      btn.textContent = w.lemma; // textContent: never parsed as HTML
      form.append(btn);
      const ru = document.createElement("span");
      ru.className = "ru";
      ru.textContent = w.expected;
      li.append(form, ru);
      list.append(li);
    }
    if (window.htmx) htmx.process(list);
    show("summary", true);
  }

  $("start").addEventListener("click", () => {
    show("start", false);
    if (items.length === 0) {
      $("status").textContent = "Не осталось слов с переводом: все выучены.";
      return;
    }
    next();
  });
  $("repeat").addEventListener("click", () => { cancelListening(); ask(); });
  $("skip").addEventListener("click", () => { cancelListening(); grade(items[index], []); });
  $("next").addEventListener("click", next);
  $("typed").addEventListener("submit", (e) => {
    e.preventDefault();
    const v = $("typed-input").value.trim();
    grade(items[index], v ? [v] : []);
  });
})();
