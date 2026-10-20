// The setup sheet: the domain, readers and issues fill every box. The state
// lives in the URL (?domain=…&readers=…&issues=…) so a filled sheet can be shared.
(function () {
  "use strict";
  var node = document.getElementById("product-data");
  if (!node) return;
  var data = JSON.parse(node.textContent).data;
  var bill = data.bill;
  var example = data.example;

  var domainInput = document.getElementById("domain");
  var readersInput = document.getElementById("readers");
  var issuesInput = document.getElementById("issues");
  var stamps = document.querySelector(".stamps");
  var reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  var hostRE = /^(?=.{1,253}$)(?!-)[a-z0-9-]{1,63}(?<!-)(\.(?!-)[a-z0-9-]{1,63}(?<!-))+$/;

  function cleanDomain(v) {
    v = String(v || "").trim().toLowerCase();
    v = v.replace(/^[a-z]+:\/\//, "").replace(/^www\./, "").replace(/[\/?#].*$/, "").replace(/\.$/, "");
    return v;
  }

  function clamp(n, lo, hi, dflt) {
    n = Math.round(Number(n));
    if (!isFinite(n) || n < lo) return dflt;
    return Math.min(n, hi);
  }

  function money(n) {
    return n.toFixed(2);
  }

  function slug(domain) {
    var first = domain.split(".")[0] || "news";
    return first.replace(/[^a-z0-9-]/g, "").slice(0, 30) || "news";
  }

  // DKIM selectors are lfr plus the year and month the keys were made.
  var now = new Date();
  var selector = "lfr" + String(now.getFullYear()).slice(2) + String(now.getMonth() + 1).padStart(2, "0");

  function render(domain, readers, issues) {
    document.querySelectorAll(".d").forEach(function (el) { el.textContent = domain; });
    document.querySelectorAll(".sel").forEach(function (el) { el.textContent = selector; });
    document.querySelectorAll(".slug").forEach(function (el) { el.textContent = slug(domain); });
    var mails = readers * issues;
    var relay = mails / 1000 * bill.sesPerThousand;
    document.getElementById("b-mails").textContent = mails.toLocaleString("en-US");
    document.getElementById("b-relay").textContent = money(relay);
    document.getElementById("b-total").textContent = money(bill.server + relay);
    document.getElementById("b-server").textContent = money(bill.server);
  }

  var pressed = "";
  var timer = 0;
  function press(domain) {
    if (reduced || !stamps || domain === pressed) return;
    pressed = domain;
    stamps.classList.remove("press");
    void stamps.offsetWidth;
    stamps.classList.add("press");
  }

  function update(fromInput) {
    var raw = cleanDomain(domainInput.value);
    var valid = hostRE.test(raw);
    domainInput.setAttribute("aria-invalid", raw && !valid ? "true" : "false");
    var domain = valid ? raw : example.domain;
    var readers = clamp(readersInput.value, 1, 10000000, example.readers);
    var issues = clamp(issuesInput.value, 1, 60, example.issues);
    render(domain, readers, issues);
    if (fromInput) {
      var q = new URLSearchParams();
      if (valid && domain !== example.domain) q.set("domain", domain);
      if (readers !== example.readers) q.set("readers", readers);
      if (issues !== example.issues) q.set("issues", issues);
      var s = q.toString();
      history.replaceState(null, "", s ? "?" + s : location.pathname);
      clearTimeout(timer);
      if (valid) timer = setTimeout(function () { press(domain); }, 600);
    }
  }

  // Restore a shared sheet.
  var params = new URLSearchParams(location.search);
  if (params.has("domain")) domainInput.value = cleanDomain(params.get("domain"));
  if (params.has("readers")) readersInput.value = clamp(params.get("readers"), 1, 10000000, example.readers);
  if (params.has("issues")) issuesInput.value = clamp(params.get("issues"), 1, 60, example.issues);
  update(false);

  [domainInput, readersInput, issuesInput].forEach(function (el) {
    el.addEventListener("input", function () { update(true); });
  });
  document.getElementById("fill").addEventListener("submit", function (e) {
    e.preventDefault();
    update(true);
    document.getElementById("records").scrollIntoView({ behavior: reduced ? "auto" : "smooth" });
  });

  document.querySelectorAll("[data-copy]").forEach(function (button) {
    var label = button.textContent;
    button.addEventListener("click", function () {
      var text = document.getElementById(button.getAttribute("data-copy")).textContent;
      var done = function () {
        button.textContent = "Copied";
        setTimeout(function () { button.textContent = label; }, 2000);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, function () { select(button); });
      } else {
        select(button);
      }
    });
  });

  // Without clipboard access, select the text so the visitor can copy it.
  function select(button) {
    var el = document.getElementById(button.getAttribute("data-copy"));
    var range = document.createRange();
    range.selectNodeContents(el);
    var sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
    button.textContent = "Selected: press Ctrl+C";
  }
})();
