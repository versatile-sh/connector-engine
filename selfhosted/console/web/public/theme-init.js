(function () {
  var saved;
  try {
    saved = window.localStorage.getItem("console.theme");
  } catch {
    saved = undefined;
  }
  var light = saved === "light" || (saved !== "dark" && window.matchMedia("(prefers-color-scheme: light)").matches);
  document.documentElement.dataset.theme = light ? "light" : "dark";
  var meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute("content", light ? "#f4f4f0" : "#0e1116");
})();
