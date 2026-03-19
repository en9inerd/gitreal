document.body.addEventListener("htmx:afterSwap", (e) => {
  if (e.detail.target.id === "result") {
    e.detail.target.scrollIntoView({ behavior: "smooth", block: "start" });
  }
});
