const BASE_URL = "http://127.0.0.1:9900";

async function api(method, path, body) {
  const opts = { method, headers: { "Content-Type": "application/json" } };
  if (body) opts.body = JSON.stringify(body);
  const res = await fetch(BASE_URL + path, opts);
  const json = await res.json();
  if (!res.ok) throw new Error(json.error || "API 오류");
  return json;
}

async function buildMenus() {
  await chrome.contextMenus.removeAll();
  const parentId = "links-parent";
  chrome.contextMenus.create({
    id: parentId,
    title: "Links에 추가",
    contexts: ["page", "link"]
  });

  try {
    const data = await api("GET", "/api/data");
    const visible = (data || []).filter(cat => !cat.hidden);

    for (const cat of visible) {
      chrome.contextMenus.create({
        id: "cat-" + cat.id,
        parentId,
        title: cat.name,
        contexts: ["page", "link"]
      });
      for (const sub of (cat.children || [])) {
        chrome.contextMenus.create({
          id: "cat-" + sub.id,
          parentId: "cat-" + cat.id,
          title: sub.name,
          contexts: ["page", "link"]
        });
      }
    }

    chrome.contextMenus.create({ id: "sep", parentId, type: "separator", contexts: ["page", "link"] });
    chrome.contextMenus.create({ id: "refresh", parentId, title: "\u27F3 \uC0C8\uB85C\uACE0\uCE68", contexts: ["page", "link"] });
  } catch (e) {
    console.error("Links \uBA54\uB274 \uAD6C\uC131 \uC2E4\uD328:", e);
    chrome.contextMenus.create({
      id: "refresh",
      parentId,
      title: "\u27F3 \uC0C8\uB85C\uACE0\uCE68 (\uC11C\uBC84 \uC5F0\uACB0 \uC2E4\uD328)",
      contexts: ["page", "link"]
    });
  }
}

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  if (info.menuItemId === "refresh") {
    buildMenus();
    return;
  }
  if (!info.menuItemId.startsWith("cat-")) return;
  // 링크 우클릭이면 링크 URL, 페이지 우클릭이면 현재 페이지 URL
  const url = info.linkUrl || tab.url;
  const title = info.linkUrl ? new URL(info.linkUrl).hostname : tab.title;
  if (!url.startsWith("http://") && !url.startsWith("https://")) return;

  const catId = parseInt(info.menuItemId.replace("cat-", ""));
  try {
    await api("POST", "/api/links", {
      category_id: catId,
      title,
      url
    });
    chrome.action.setBadgeText({ text: "OK", tabId: tab.id });
    chrome.action.setBadgeBackgroundColor({ color: "#2d9a46" });
    setTimeout(() => chrome.action.setBadgeText({ text: "", tabId: tab.id }), 2000);
  } catch (e) {
    console.error("Links \uC800\uC7A5 \uC2E4\uD328:", e);
    chrome.action.setBadgeText({ text: "ERR", tabId: tab.id });
    chrome.action.setBadgeBackgroundColor({ color: "#f85149" });
    setTimeout(() => chrome.action.setBadgeText({ text: "", tabId: tab.id }), 3000);
  }
});

chrome.runtime.onInstalled.addListener(buildMenus);
chrome.runtime.onStartup.addListener(buildMenus);
