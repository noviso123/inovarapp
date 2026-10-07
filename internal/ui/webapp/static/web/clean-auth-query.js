(() => {
  const url = new URL(window.location.href);
  let changed = false;
  for (const key of [...url.searchParams.keys()]) {
    if (["email", "password", "senha"].includes(key.toLowerCase())) {
      url.searchParams.delete(key);
      changed = true;
    }
  }
  if (changed) {
    window.history.replaceState(window.history.state, "", url.pathname + url.search + url.hash);
  }
})();
