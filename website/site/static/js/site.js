(function () {
  "use strict";
  var KEY = "rg-docs-theme";

  function storedChoice() {
    try {
      return localStorage.getItem(KEY) || "system";
    } catch (e) {
      return "system";
    }
  }

  function apply(choice) {
    if (choice !== "light" && choice !== "dark" && choice !== "system") {
      choice = "system";
    }
    var dark = choice === "dark" ||
      (choice === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
    var el = document.documentElement;
    el.dataset.theme = dark ? "dark" : "light";
    el.dataset.themeChoice = choice;
    el.style.colorScheme = dark ? "dark" : "light";
    document.querySelectorAll("[data-set-theme]").forEach(function (b) {
      var on = b.getAttribute("data-set-theme") === choice;
      b.setAttribute("aria-checked", on ? "true" : "false");
      b.classList.toggle("active", on);
    });
  }

  // Dropdown menus. The <details> element already opens natively without
  // JS. With JS we add ARIA state, keyboard navigation, Escape handling,
  // focus return, and close-on-select.
  var menus = Array.prototype.slice.call(document.querySelectorAll("details.menu"));

  function menuItems(d) {
    return Array.prototype.slice.call(
      d.querySelectorAll("[role='menuitem'], [role='menuitemradio'], .menu-list a, .menu-list button")
    );
  }

  function summaryOf(d) {
    return d.querySelector("summary");
  }

  function closeMenu(d, refocus) {
    if (d.open) {
      d.open = false;
    }
    if (refocus) {
      summaryOf(d).focus();
    }
  }

  menus.forEach(function (d) {
    var sum = summaryOf(d);
    sum.setAttribute("aria-haspopup", "menu");
    sum.setAttribute("aria-expanded", "false");
    d.addEventListener("toggle", function () {
      sum.setAttribute("aria-expanded", d.open ? "true" : "false");
      if (d.open) {
        menus.forEach(function (o) {
          if (o !== d && o.open) {
            closeMenu(o, false);
          }
        });
      }
    });
    sum.addEventListener("keydown", function (ev) {
      if (ev.key === "ArrowDown" || ev.key === "ArrowUp") {
        ev.preventDefault();
        if (!d.open) {
          d.open = true;
        }
        var items = menuItems(d);
        if (items.length) {
          (ev.key === "ArrowDown" ? items[0] : items[items.length - 1]).focus();
        }
      }
    });
    d.addEventListener("keydown", function (ev) {
      var items = menuItems(d);
      var i = items.indexOf(document.activeElement);
      if (ev.key === "Escape") {
        ev.preventDefault();
        closeMenu(d, true);
      } else if (i >= 0 && ev.key === "ArrowDown") {
        ev.preventDefault();
        items[(i + 1) % items.length].focus();
      } else if (i >= 0 && ev.key === "ArrowUp") {
        ev.preventDefault();
        items[(i - 1 + items.length) % items.length].focus();
      } else if (i >= 0 && ev.key === "Home") {
        ev.preventDefault();
        items[0].focus();
      } else if (i >= 0 && ev.key === "End") {
        ev.preventDefault();
        items[items.length - 1].focus();
      } else if (ev.key === "Tab" && d.open) {
        closeMenu(d, false);
      }
    });
    d.addEventListener("click", function (ev) {
      var hit = ev.target.closest("[role='menuitem'], [role='menuitemradio']");
      if (hit) {
        closeMenu(d, false);
      }
    });
  });

  document.addEventListener("click", function (ev) {
    menus.forEach(function (d) {
      if (d.open && !d.contains(ev.target)) {
        closeMenu(d, false);
      }
    });
  });
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") {
      menus.forEach(function (d) {
        if (d.open) {
          closeMenu(d, true);
        }
      });
    }
  });

  // Theme choice.
  document.querySelectorAll("[data-set-theme]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var c = btn.getAttribute("data-set-theme");
      try {
        localStorage.setItem(KEY, c);
      } catch (e) { /* private mode */ }
      apply(c);
    });
  });
  apply(storedChoice());

  try {
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", function () {
      if (storedChoice() === "system") {
        apply("system");
      }
    });
  } catch (e) { /* older engines */ }

  // Mobile sidebar toggle.
  var menuBtn = document.getElementById("menu-btn");
  var sidebar = document.getElementById("sidebar");
  if (menuBtn && sidebar) {
    menuBtn.addEventListener("click", function () {
      var open = document.documentElement.classList.toggle("sidebar-open");
      menuBtn.setAttribute("aria-expanded", open ? "true" : "false");
    });
    sidebar.addEventListener("click", function (ev) {
      if (ev.target.closest("a")) {
        document.documentElement.classList.remove("sidebar-open");
        menuBtn.setAttribute("aria-expanded", "false");
      }
    });
  }

  // Copy buttons. The button markup is rendered by the codeblock render
  // hook; JS only wires the click.
  document.querySelectorAll(".copy-btn").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var block = btn.closest(".codeblock");
      var code = block ? block.querySelector("pre code") : null;
      if (!code) {
        return;
      }
      var text = code.innerText;
      var status = btn.querySelector(".cb-status");
      function done(ok) {
        btn.classList.add("copied");
        if (status) {
          status.textContent = ok ? "Copied" : "Copy failed";
        }
        setTimeout(function () {
          btn.classList.remove("copied");
          if (status) {
            status.textContent = "";
          }
        }, 1500);
      }
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(function () {
          done(true);
        }, function () {
          legacyCopy(text);
          done(true);
        });
      } else {
        done(legacyCopy(text));
      }
    });
  });

  function legacyCopy(text) {
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    var ok = false;
    try {
      ok = document.execCommand("copy");
    } catch (e) { /* leave false */ }
    document.body.removeChild(ta);
    return ok;
  }
})();
