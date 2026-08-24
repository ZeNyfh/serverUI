const username = document.querySelector("#username");
const settingsButton = document.querySelector("#settings-button");
const logoutButton = document.querySelector("#logout-button");
const sidebarItems = document.querySelector("#sidebar-items");
const cardsContainer = document.querySelector("#cards-container");
let settingsPanel;
let settingsUsername;
let settingsMessage;
let profileImageMessage;
let terminal;
let terminalSocket;
let terminalFit;
let terminalResizeObserver;

const items = [
  {
    id: "console",
    name: "Console",
    image: "/assets/console.svg",
  },
  {
    id: "immich",
    name: "Immich",
    image: "/assets/immich.png",
  },
];

function renderItems() {
  sidebarItems.innerHTML = items
    .map(
      (item) => `
        <button class="sidebar-item" type="button" data-item="${item.id}">
          <img src="${item.image}" alt="" />
          <span>${item.name}</span>
        </button>`,
    )
    .join("");

  cardsContainer.innerHTML = items
    .map(
      (item) => `
        <button class="item-card ${item.id}-card" type="button" data-item="${item.id}" style="background-image: url('${item.image}')">
          <span>${item.name}</span>
        </button>`,
    )
    .join("");

  document.querySelectorAll("[data-item]").forEach((element) => {
    element.addEventListener("click", () => selectItem(element.dataset.item));
  });
}

function selectItem(itemID) {
  if (itemID === "console") showConsole();
  if (itemID === "immich") showImmich();
}

function showImmich() {
  cardsContainer.classList.add("embed-active");
  const immichURL = `https://${location.hostname}:2283`;
  cardsContainer.innerHTML = `<section class="embed-view"><div class="console-toolbar"><button id="immich-back" type="button">Back</button><strong>Immich</strong></div><iframe src="${immichURL}" title="Immich" allow="fullscreen"></iframe></section>`;
  document.querySelector("#immich-back").addEventListener("click", () => {
    cardsContainer.classList.remove("embed-active");
    renderItems();
  });
}

async function showConsole() {
  cardsContainer.classList.add("console-active");
  cardsContainer.innerHTML = `<section class="console-view"><div class="console-toolbar"><button id="console-back" type="button">Back</button><strong>Console</strong></div><p id="console-status"></p><div id="console-sessions" class="console-sessions"></div><div id="terminal" hidden></div></section>`;
  document.querySelector("#console-back").addEventListener("click", () => {
    if (terminal) {
      closeTerminal();
      showConsole();
      return;
    }
    cardsContainer.classList.remove("console-active");
    renderItems();
  });
  loadConsoleSessions();
}

async function loadConsoleSessions() {
  const status = document.querySelector("#console-status");
  const sessions = document.querySelector("#console-sessions");
  status.textContent = "Loading tmux sessions…";
  try {
    const response = await fetch("/api/console/sessions", { method: "POST" });
    if (!response.ok) throw new Error(await response.text());
    const tmuxSessions = await response.json();
    if (tmuxSessions.length === 0) {
      status.textContent = "No tmux sessions found. Start a new one:";
      renderSessionCards(sessions, []);
      return;
    }
    status.textContent = "Choose a tmux session or start a new one:";
    renderSessionCards(sessions, tmuxSessions);
  } catch (error) {
    status.textContent = `Could not connect to SSH: ${error.message}`;
  }
}

function renderSessionCards(container, tmuxSessions) {
  container.replaceChildren();
  tmuxSessions.forEach((session) => {
    const card = document.createElement("article");
    card.className = "console-session-card";
    const title = document.createElement("input");
    title.className = "session-title";
    title.value = session.name;
    title.dataset.session = session.name;
    title.setAttribute("aria-label", `Rename tmux session ${session.name}`);
    const preview = document.createElement("button");
    preview.className = "terminal-preview";
    preview.type = "button";
    preview.innerHTML = new AnsiUp().ansi_to_html(session.preview || "No terminal output available");
    preview.addEventListener("click", () => openTerminal(title.dataset.session));
    title.addEventListener("change", () => renameSession(title, preview));
    title.addEventListener("keydown", (event) => {
      if (event.key === "Enter") title.blur();
    });
    card.append(title, preview);
    container.append(card);
    fitTerminalPreview(preview);
  });

  const startCard = document.createElement("button");
  startCard.className = "console-session-card start-session";
  startCard.type = "button";
  startCard.textContent = "Start new";
  startCard.addEventListener("click", () => openTerminal());
  container.append(startCard);
}

function fitTerminalPreview(preview) {
  const lines = preview.textContent.split("\n");
  const widestLine = Math.max(...lines.map((line) => line.length), 1);
  const fontSize = Math.max(3, Math.min(11, (preview.clientWidth - 20) / (widestLine * 0.62), (preview.clientHeight - 20) / (lines.length * 1.2)));
  preview.style.fontSize = `${fontSize}px`;
}

async function renameSession(title, preview) {
  const oldName = title.dataset.session;
  const newName = title.value.trim();
  if (newName === oldName) return;
  const response = await fetch("/api/console/sessions/rename", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ oldName, newName }),
  });
  if (!response.ok) {
    title.value = oldName;
    document.querySelector("#console-status").textContent = await response.text();
    return;
  }
  title.dataset.session = newName;
}

function openTerminal(session) {
  const status = document.querySelector("#console-status");
  const sessions = document.querySelector("#console-sessions");
  const terminalElement = document.querySelector("#terminal");
  document.querySelector(".console-toolbar strong").hidden = true;
  status.hidden = true;
  sessions.hidden = true;
  terminalElement.hidden = false;

  terminal = new Terminal({ cursorBlink: true, fontFamily: "monospace", fontSize: 15, theme: { background: "#111", foreground: "#f5f5f5" } });
  terminalFit = new FitAddon.FitAddon();
  terminal.loadAddon(terminalFit);
  terminal.open(terminalElement);
  terminalFit.fit();
  terminalResizeObserver = new ResizeObserver(() => terminalFit?.fit());
  terminalResizeObserver.observe(terminalElement);
  requestAnimationFrame(() => terminalFit?.fit());
  terminal.focus();
  const protocol = location.protocol === "https:" ? "wss" : "ws";
  terminalSocket = new WebSocket(`${protocol}://${location.host}/api/console/terminal${session ? `?session=${encodeURIComponent(session)}` : ""}`);
  terminalSocket.binaryType = "arraybuffer";
  terminalSocket.addEventListener("open", () => {
    terminalSocket.send(JSON.stringify({ type: "resize", cols: terminal.cols, rows: terminal.rows }));
  });
  terminalSocket.addEventListener("message", (event) => {
    const output = typeof event.data === "string" ? event.data : new TextDecoder().decode(event.data);
    terminal.write(output);
  });
  terminalSocket.addEventListener("close", () => terminal?.write("\r\n\x1b[31mSSH connection closed.\x1b[0m\r\n"));
  terminal.onData((data) => terminalSocket?.readyState === WebSocket.OPEN && terminalSocket.send(JSON.stringify({ type: "input", data })));
  terminal.onResize(({ cols, rows }) => terminalSocket?.readyState === WebSocket.OPEN && terminalSocket.send(JSON.stringify({ type: "resize", cols, rows })));
}

function closeTerminal() {
  terminalSocket?.close();
  terminalSocket = undefined;
  terminal?.dispose();
  terminal = undefined;
  terminalResizeObserver?.disconnect();
  terminalResizeObserver = undefined;
  terminalFit = undefined;
}

window.addEventListener("resize", () => terminalFit?.fit());

async function loadAccount() {
  const response = await fetch("/api/me");
  if (!response.ok) {
    window.location.reload();
    return;
  }

  const account = await response.json();
  username.textContent = account.username;
  if (settingsUsername) settingsUsername.value = account.username;
}

function loadProfileImage() {
  const image = document.querySelector("#profile-image");
  image.onerror = () => {
    image.onerror = null;
    image.src = "/assets/profile-placeholder.svg";
  };
  image.src = `/api/profile-image?updated=${Date.now()}`;
}

async function showSettings() {
  if (terminal) closeTerminal();
  cardsContainer.classList.remove("console-active", "embed-active");
  const response = await fetch("/settings.html");
  if (!response.ok) {
    window.location.reload();
    return;
  }

  cardsContainer.innerHTML = `<section class="settings-view"><div class="console-toolbar"><button id="settings-back" type="button">Back</button><strong>Settings</strong></div>${await response.text()}</section>`;
  settingsPanel = document.querySelector("#settings-panel");
  settingsPanel.hidden = false;
  settingsUsername = document.querySelector("#settings-username");
  settingsMessage = document.querySelector("#settings-message");
  profileImageMessage = document.querySelector("#profile-image-message");

  document.querySelector("#settings-back").addEventListener("click", renderItems);
  document.querySelector("#settings-form").addEventListener("submit", updateAccount);
  document.querySelector("#profile-image-form").addEventListener("submit", uploadProfileImage);
  loadAccount();
  loadProfileImage();
}

async function updateAccount(event) {
  event.preventDefault();
  const response = await fetch("/api/account", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      username: settingsUsername.value,
      currentPassword: document.querySelector("#current-password").value,
      newPassword: document.querySelector("#new-password").value,
    }),
  });

  if (!response.ok) {
    settingsMessage.textContent = await response.text();
    return;
  }

  document.querySelector("#current-password").value = "";
  document.querySelector("#new-password").value = "";
  settingsMessage.textContent = "Changes saved.";
  loadAccount();
}

async function uploadProfileImage(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const response = await fetch("/api/profile-image", { method: "POST", body: new FormData(form) });
  if (!response.ok) {
    profileImageMessage.textContent = await response.text();
    return;
  }
  form.reset();
  profileImageMessage.textContent = "Profile picture updated.";
  loadProfileImage();
}

async function logout() {
  const response = await fetch("/api/logout", { method: "POST" });
  if (response.ok) window.location.reload();
}

async function initialize() {
  renderItems();
  settingsButton.addEventListener("click", showSettings);
  logoutButton.addEventListener("click", logout);
  await Promise.all([loadAccount(), loadProfileImage()]);
}

initialize();
