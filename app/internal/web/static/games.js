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
  let round = 0; // bumped by every ask and every grade: stale awaits bail out
  let current = null; // the running recognition, if any
  let audio = null; // the prompt being played, if any
  let typing = !Recognition; // no recognition, or the microphone failed
  const wrong = [];

  // Recognition errors after which listening again is pointless.
  const fatal = new Set(["not-allowed", "service-not-allowed", "audio-capture", "network", "language-not-supported"]);

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

  function stopPrompt() {
    if (audio) { audio.pause(); audio = null; }
    if ("speechSynthesis" in window) speechSynthesis.cancel();
  }

  // Some browsers never fire "end" (no voices, blocked audio): stop the
  // prompt and move on after a limit that grows with the text, so the
  // microphone never opens over the site's own voice.
  function sayPrompt(item) {
    const my = round; // a late timer of an old prompt must not stop a new one
    const limit = 4000 + 120 * item.say.length;
    const playing = item.audio
      ? new Promise((done) => {
        audio = new Audio(item.audio);
        audio.onended = done;
        audio.onerror = () => speakText(item).then(done);
        audio.play().catch(() => speakText(item).then(done));
      })
      : speakText(item);
    return Promise.race([playing, new Promise((done) => setTimeout(done, limit))])
      .then(() => { if (my === round) stopPrompt(); });
  }

  // Resolves with {heard, error}: the recognised alternatives and the
  // recognition error code, if any; or null when cancelled.
  function listen() {
    return new Promise((done) => {
      const r = new Recognition();
      r.lang = answerLang;
      r.maxAlternatives = 5;
      r.interimResults = false;
      let heard = [];
      let error = "";
      r.onresult = (e) => { heard = Array.from(e.results[0], (alt) => alt.transcript); };
      r.onerror = (e) => { error = e.error || "unknown"; };
      r.onend = () => { if (current === r) { current = null; done({ heard, error }); } else done(null); };
      current = r;
      $("status").textContent = "🎤 Говорите…";
      try {
        r.start();
      } catch (e) {
        current = null;
        done({ heard: [], error: "not-allowed" });
      }
    });
  }

  function askTyped() {
    $("status").textContent = "Введите ответ";
    show("typed", true);
    $("typed-input").value = "";
    $("typed-input").focus();
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
    show("typed", false);
    const my = ++round;
    $("status").textContent = "🔊 Слушайте…";
    await sayPrompt(item);
    if (my !== round) return; // skipped or repeated meanwhile
    if (typing) return askTyped();
    const res = await listen();
    if (res === null || my !== round) return;
    if (fatal.has(res.error)) {
      typing = true;
      $("no-mic").textContent = "Микрофон недоступен (нет разрешения или устройства) — отвечайте текстом.";
      show("no-mic", true);
      return askTyped();
    }
    if (res.heard.length === 0 && res.error) {
      $("status").textContent = "Ничего не слышно — нажмите «Повторить» или «Не знаю».";
      return;
    }
    await grade(item, res.heard);
  }

  async function grade(item, answers) {
    round += 1; // an answer closes the round: late awaits from it must not act
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
  $("repeat").addEventListener("click", () => { stopPrompt(); cancelListening(); ask(); });
  $("skip").addEventListener("click", () => { stopPrompt(); cancelListening(); grade(items[index], []); });
  $("next").addEventListener("click", next);
  $("typed").addEventListener("submit", (e) => {
    e.preventDefault();
    const v = $("typed-input").value.trim();
    grade(items[index], v ? [v] : []);
  });
})();
