const form = document.querySelector("#auth-form");
const submitButton = document.querySelector("#submit-button");
const switchButton = document.querySelector("#switch-button");
const message = document.querySelector("#message");

let signingUp = false;

switchButton.addEventListener("click", () => {
  signingUp = !signingUp;
  submitButton.textContent = signingUp ? "Sign up" : "Log in";
  switchButton.textContent = signingUp ? "Back to login" : "Create an account";
  message.textContent = "";
});

form.addEventListener("submit", async (event) => {
  event.preventDefault();

  const credentials = {
    username: document.querySelector("#username").value,
    password: document.querySelector("#password").value,
  };
  const endpoint = signingUp ? "/api/users" : "/api/login";
  const response = await fetch(endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(credentials),
  });

  if (response.ok && !signingUp) {
    window.location.reload();
    return;
  }

  if (response.ok) {
    switchButton.click();
    message.textContent = "Account created. You can now log in.";
    return;
  }

  message.textContent = await response.text();
});
