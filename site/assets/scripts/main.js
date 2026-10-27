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
  var records = document.getElementById("record-list");
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
    document.getElementById("readers-out").textContent = readers.toLocaleString("en-US");
    document.getElementById("issues-out").textContent = issues;
    var scale = 490 / Math.max(49, bill.server + relay);
    document.querySelectorAll(".bars rect").forEach(function (bar, i) {
      bar.setAttribute("width", [bill.server + relay, 49, 9, 33, 35][i] * scale);
    });
    document.querySelector(".chart-labels text").textContent = "lfr · $" + money(bill.server + relay);
    document.getElementById("tier-note").textContent =
      "beehiiv: $49 at 1,000 readers, billed yearly" + (readers > 1000 ? " (above the stated tier). " : " (stated tier). ") +
      "Buttondown: " + (readers <= 100 ? "free to 100. " : "above the free tier; $9 / $29 paid tier limits unstated. ") +
      "Kit: " + (readers <= 10000 ? "free to 10,000; paid from $33. " : "above the stated free tier; paid from $33. ") +
      "Ghost: software free, Mailgun from $35; server extra. Bars show stated paid prices, not quotes for your list.";
  }

  var shownDomain = "";
  function highlight(domain) {
    if (domain === shownDomain) return;
    shownDomain = domain;
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    records.querySelectorAll("li:has(.d)").forEach(function (row) {
      row.classList.remove("changed");
      void row.offsetWidth;
      row.classList.add("changed");
    });
  }

  function update(fromInput) {
    var raw = cleanDomain(domainInput.value);
    var valid = hostRE.test(raw);
    domainInput.setAttribute("aria-invalid", raw && !valid ? "true" : "false");
    var domain = valid ? raw : example.domain;
    var readers = clamp(readersInput.value, 1, 15000, example.readers);
    var issues = clamp(issuesInput.value, 1, 60, example.issues);
    render(domain, readers, issues);
    if (fromInput) {
      var q = new URLSearchParams();
      if (valid && domain !== example.domain) q.set("domain", domain);
      if (readers !== example.readers) q.set("readers", readers);
      if (issues !== example.issues) q.set("issues", issues);
      var s = q.toString();
      history.replaceState(null, "", s ? "?" + s : location.pathname);
      if (valid) highlight(domain);
    }
  }

  // Restore a shared sheet.
  var params = new URLSearchParams(location.search);
  if (params.has("domain")) domainInput.value = cleanDomain(params.get("domain"));
  if (params.has("readers")) readersInput.value = clamp(params.get("readers"), 1, 15000, example.readers);
  if (params.has("issues")) issuesInput.value = clamp(params.get("issues"), 1, 60, example.issues);
  update(false);
  shownDomain = cleanDomain(domainInput.value);

  [domainInput, readersInput, issuesInput].forEach(function (el) {
    el.addEventListener("input", function () { update(true); });
  });
  document.getElementById("replay").addEventListener("click", function replay() {
    var terminal = document.getElementById("terminal");
    var clone = terminal.cloneNode(true);
    terminal.replaceWith(clone);
    clone.querySelector("#replay").addEventListener("click", replay);
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
