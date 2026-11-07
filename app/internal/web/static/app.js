// Small behaviours on top of HTMX: pronunciation and the word dialog.
"use strict";

function speak(btn) {
  const url = btn.dataset.audio;
  const fallback = () => {
    if (!("speechSynthesis" in window)) return;
    const u = new SpeechSynthesisUtterance(btn.dataset.text);
    u.lang = btn.dataset.lang || "en-GB";
    speechSynthesis.cancel();
    speechSynthesis.speak(u);
  };
  if (!url) return fallback();
  new Audio(url).play().catch(fallback);
}

const wide = window.matchMedia("(min-width: 768px)");

document.addEventListener("click", (ev) => {
  const say = ev.target.closest(".say");
  if (say) {
    speak(say);
    return;
  }

  // Wide screens open a word in a dialog; phones follow the link to a page.
  const ex = ev.target.closest("a.ex");
  if (ex && wide.matches && window.htmx) {
    ev.preventDefault();
    const dlg = document.getElementById("word-dialog");
    htmx
      .ajax("GET", ex.getAttribute("href"), { target: "#word-dialog-body", swap: "innerHTML" })
      .then(() => dlg.showModal());
    return;
  }

  if (ev.target.closest("[data-close]")) {
    ev.target.closest("dialog")?.close();
    return;
  }

  // A click on the backdrop (the dialog element itself) closes it.
  if (ev.target.id === "word-dialog") ev.target.close();
});
