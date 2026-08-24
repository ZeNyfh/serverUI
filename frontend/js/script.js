const username = document.querySelector("#username");
const settingsButton = document.querySelector("#settings-button");
const sidebarItems = document.querySelector("#sidebar-items");
const cardsContainer = document.querySelector("#cards-container");
let settingsPanel;
let settingsUsername;
let settingsMessage;

const items = [
  {
    id: "placeholder",
    name: "Placeholder",
    image: "/assets/placeholder.png",
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
        <button class="item-card" type="button" data-item="${item.id}" style="background-image: url('${item.image}')">
          <span>${item.name}</span>
        </button>`,
    )
    .join("");

  document.querySelectorAll("[data-item]").forEach((element) => {
    element.addEventListener("click", () => selectItem(element.dataset.item));
  });
}

function selectItem(itemID) {
  void itemID;
}

async function loadAccount() {
  const response = await fetch("/api/me");
  if (!response.ok) {
    window.location.reload();
    return;
  }

  const account = await response.json();
  username.textContent = account.username;
  settingsUsername.value = account.username;
}

async function loadSettingsPanel() {
  const response = await fetch("/settings.html");
  if (!response.ok) {
    window.location.reload();
    return;
  }

  document.querySelector("#settings-container").innerHTML = await response.text();
  settingsPanel = document.querySelector("#settings-panel");
  settingsUsername = document.querySelector("#settings-username");
  settingsMessage = document.querySelector("#settings-message");

  settingsButton.addEventListener("click", () => {
    settingsPanel.hidden = !settingsPanel.hidden;
    settingsMessage.textContent = "";
  });

  document.querySelector("#settings-form").addEventListener("submit", updateAccount);
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

async function initialize() {
  renderItems();
  await loadSettingsPanel();
  await loadAccount();
}

initialize();
