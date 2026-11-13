#!/usr/bin/env node
/**
 * Insight Windows desktop shell.
 *
 * CreatorHub uses a WebView2 wrapper.  Insight keeps the same user-facing
 * property on Windows by using the installed Edge WebView2/Edge engine in
 * app mode with a private, persistent profile.  The small bridge below also
 * registers that browser as an Insight edge node, so the existing ingest and
 * monitoring code can call the authenticated 视频号助手 page directly.
 */
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { spawn } from "node:child_process";
import fs from "node:fs";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const apiURL = (process.env.INSIGHT_API_URL || "http://127.0.0.1:18081").replace(/\/$/, "");
const cdpPort = Number(process.env.INSIGHT_DESKTOP_CDP_PORT || 19222);
const bridgePort = Number(process.env.INSIGHT_DESKTOP_BRIDGE_PORT || 18765);
const clientID = process.env.INSIGHT_EDGE_CLIENT_ID || `insight-desktop-${os.hostname()}-${process.pid}`;
const edgeSecret = (process.env.INSIGHT_EDGE_WS_SECRET || "").trim();
const loginURL = "https://channels.weixin.qq.com/platform/login";
const profileDir = path.resolve(process.env.INSIGHT_DESKTOP_PROFILE || path.join(root, "var", "insight-browser"));

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const asObject = (value) => {
  if (value && typeof value === "object") return value;
  if (typeof value !== "string" || !value.trim()) return {};
  try { return JSON.parse(value); } catch { return {}; }
};

async function waitForHealth() {
  const deadline = Date.now() + 30000;
  let lastError = "";
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${apiURL}/api/healthz`, { signal: AbortSignal.timeout(2500) });
      if (response.ok) return;
      lastError = `HTTP ${response.status}`;
    } catch (error) {
      lastError = error instanceof Error ? error.message : String(error);
    }
    await sleep(500);
  }
  throw new Error(`Insight API 未就绪（${lastError || "超时"}），请先启动 API。`);
}

function locateEdge() {
  const candidates = [
    process.env.INSIGHT_EDGE_EXE,
    "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
    "C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe",
  ].filter(Boolean);
  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) return candidate;
  }
  return "msedge.exe";
}

class CDPPage {
  constructor(target) {
    this.target = target;
    this.nextID = 1;
    this.pending = new Map();
    this.opened = false;
    this.socket = new WebSocket(target.webSocketDebuggerUrl);
    this.ready = new Promise((resolve, reject) => {
      this.socket.addEventListener("open", () => { this.opened = true; resolve(); });
      this.socket.addEventListener("error", () => reject(new Error("Edge DevTools 连接失败")));
    });
    this.socket.addEventListener("message", (event) => {
      let message;
      try { message = JSON.parse(String(event.data)); } catch { return; }
      if (!message.id) return;
      const waiter = this.pending.get(message.id);
      if (!waiter) return;
      this.pending.delete(message.id);
      if (message.error) waiter.reject(new Error(message.error.message || "DevTools 调用失败"));
      else waiter.resolve(message.result);
    });
    this.socket.addEventListener("close", () => {
      for (const waiter of this.pending.values()) waiter.reject(new Error("Edge 页面已关闭"));
      this.pending.clear();
      this.opened = false;
    });
  }

  async command(method, params = {}) {
    await this.ready;
    if (!this.opened) throw new Error("Edge 页面未连接");
    const id = this.nextID++;
    const result = new Promise((resolve, reject) => this.pending.set(id, { resolve, reject }));
    this.socket.send(JSON.stringify({ id, method, params }));
    return result;
  }

  async evaluate(expression) {
    const result = await this.command("Runtime.evaluate", {
      expression,
      awaitPromise: true,
      returnByValue: true,
      userGesture: true,
    });
    if (result?.exceptionDetails) throw new Error(result.exceptionDetails.text || "页面脚本执行失败");
    return result?.result?.value;
  }

  async navigate(url) {
    await this.command("Page.enable");
    return this.command("Page.navigate", { url });
  }

  close() {
    try { this.socket.close(); } catch {}
  }
}

class InsightDesktopBridge {
  constructor() {
    this.edgeProcess = null;
    this.loginProcess = null;
    this.page = null;
    this.lastState = {
      status: "offline",
      pagePath: "",
      href: "",
      finderUsername: "",
      loginQRCode: "",
      loginQRCodeURL: "",
      apiReady: false,
      apiFunctional: false,
      methods: {},
      timestamp: Date.now(),
    };
    this.edgeSocket = null;
    this.edgeOpen = false;
    this.edgePending = new Map();
    this.edgeHeartbeatTimer = null;
    this.stateTimer = null;
  }

  async start() {
    await waitForHealth();
    const fs = await import("node:fs");
    fs.mkdirSync(profileDir, { recursive: true });
    this.edgeProcess = spawn(locateEdge(), [
      `--app=${apiURL}/#platform`,
      `--user-data-dir=${profileDir}`,
      `--remote-debugging-port=${cdpPort}`,
      "--no-first-run",
      "--no-default-browser-check",
      "--disable-features=msEdgeSidebarV2",
    ], { windowsHide: false, stdio: "ignore" });
    this.edgeProcess.once("exit", () => this.stop());
    this.stateTimer = setInterval(() => this.refreshPageState().catch(() => {}), 1500);
    await this.connectEdgeNode();
    // Edge may still be bringing up the CDP endpoint after the app window
    // starts.  A transient discovery failure must not tear down the bridge;
    // the state timer will retry and publish the authenticated page once it
    // becomes available.
    await this.refreshPageState().catch(() => {});
    console.log(`Insight 内置浏览器已启动: ${apiURL}/#platform`);
    console.log(`登录 Profile: ${profileDir}`);
  }

  async discoverTargets() {
    const response = await fetch(`http://127.0.0.1:${cdpPort}/json/list`, { signal: AbortSignal.timeout(1500) });
    if (!response.ok) throw new Error(`CDP HTTP ${response.status}`);
    return response.json();
  }

  async findChannelPage() {
    const targets = await this.discoverTargets();
    const candidate = targets.find((target) => target.type === "page" && /^https:\/\/channels\.weixin\.qq\.com\//i.test(target.url || ""));
    if (!candidate?.webSocketDebuggerUrl) return null;
    if (this.page?.target?.id === candidate.id && this.page.opened) return this.page;
    this.page?.close();
    this.page = new CDPPage(candidate);
    await this.page.ready;
    await this.page.command("Runtime.enable").catch(() => {});
    return this.page;
  }

  async openLogin() {
    const page = await this.findChannelPage();
    if (page) {
      await page.navigate(loginURL);
      return;
    }
    this.loginProcess = spawn(locateEdge(), [
      "--new-window",
      `--app=${loginURL}`,
      `--user-data-dir=${profileDir}`,
      `--remote-debugging-port=${cdpPort}`,
      "--no-first-run",
      "--no-default-browser-check",
    ], { windowsHide: false, stdio: "ignore" });
  }

  async refreshPageState() {
    const page = await this.findChannelPage();
    if (!page) {
      this.lastState = {
        ...this.lastState,
        status: "login_required",
        pagePath: "",
        href: "",
        finderUsername: "",
        loginQRCode: "",
        loginQRCodeURL: "",
        apiReady: false,
        methods: {},
        timestamp: Date.now(),
      };
      return this.lastState;
    }
    const state = await page.evaluate(`(() => {
      const visible = (el) => {
        if (!el) return false;
        const r = el.getBoundingClientRect();
        const s = getComputedStyle(el);
        return r.width >= 140 && r.height >= 140 && r.width <= 720 && r.height <= 720 && s.display !== 'none' && s.visibility !== 'hidden';
      };
      const candidates = [];
      const add = (value, el, bonus = 0) => {
        const raw = String(value || '').trim();
        if (!raw || raw.length > 320000 || !visible(el)) return;
        const r = el.getBoundingClientRect();
        if (Math.abs(r.width - r.height) > Math.max(24, Math.min(r.width, r.height) * .28)) return;
        const hint = String(el.className || '') + ' ' + String(el.id || '') + ' ' + String(el.alt || '');
        candidates.push({ value: raw, score: r.width * r.height + (/qr|qrcode|scan|code|登录|扫码/i.test(hint) ? 1000000 : 0) + bonus });
      };
      document.querySelectorAll('img').forEach((el) => add(el.currentSrc || el.src || el.getAttribute('data-src'), el, String(el.src || '').startsWith('data:image/') ? 300000 : 0));
      document.querySelectorAll('canvas').forEach((el) => { try { add(el.toDataURL('image/png'), el, 500000); } catch {} });
      document.querySelectorAll('*').forEach((el) => { const bg = getComputedStyle(el).backgroundImage || ''; const m = bg.match(/url\\(["']?([^"')]+)["']?\\)/i); if (m) add(m[1], el, 100000); });
      candidates.sort((a, b) => b.score - a.score);
      const text = String(document.body?.innerText || document.body?.textContent || '');
      const path = String(location.pathname || '');
      const match = text.match(/视频号\\s*ID\\s*[:：]\\s*([A-Za-z0-9_-]{5,})/i);
      const methods = {
        finderUserPage: !!(window.WXU?.API && typeof window.WXU.API.finderUserPage === 'function'),
        finderSearch: !!(window.WXU?.API2 && typeof window.WXU.API2.finderSearch === 'function'),
        finderGetCommentDetail: !!(window.WXU?.API && typeof window.WXU.API.finderGetCommentDetail === 'function'),
        finderGetCommentList: !!(window.WXU?.API && typeof window.WXU.API.finderGetCommentList === 'function')
      };
      const previousUsername = ${JSON.stringify(this.lastState.finderUsername || '')};
      const username = window.__wx_username || match?.[1] || (path.endsWith('/platform/post/list') ? previousUsername : '');
      const frames = Array.from(document.querySelectorAll('iframe')).map((el) => ({ src: el.src || '', w: el.getBoundingClientRect().width, h: el.getBoundingClientRect().height }));
      const qrFrame = frames.find((frame) => frame.w >= 180 && frame.h >= 140 && /qrconnect|oauth|login|appid/i.test(frame.src));
      return { pagePath: path, href: location.href, finderUsername: String(username).trim(), loginQRCode: username ? '' : (candidates[0]?.value || ''), loginQRCodeURL: username ? '' : (qrFrame?.src || ''), apiReady: Object.values(methods).some(Boolean), methods, visible: !document.hidden, timestamp: Date.now() };
    })()`);
    // The current assistant build exposes the post list through its normal
    // page request but no longer leaves WXU on window.  Advertise the
    // compatible feed capability so Insight can use that first-party page
    // request as the transport fallback.
    const fallbackMethods = state.finderUsername
      ? { ...state.methods, finderUserPage: true }
      : state.methods;
    this.lastState = {
      ...state,
      apiReady: !!state.apiReady || !!state.finderUsername,
      methods: fallbackMethods,
      status: state.finderUsername ? "authenticated" : "login_required",
    };
    return this.lastState;
  }

  async connectEdgeNode() {
    const url = new URL(`${apiURL.replace(/^http/, "ws")}/ws/client`);
    url.searchParams.set("client_id", clientID);
    if (edgeSecret) url.searchParams.set("secret", edgeSecret);
    this.edgeSocket = new WebSocket(url);
    this.edgeSocket.addEventListener("open", () => {
      this.edgeOpen = true;
      this.sendHeartbeat();
      this.edgeHeartbeatTimer = setInterval(() => this.sendHeartbeat(), 10000);
      console.log(`Insight 内置浏览器已连接 Edge 网关: ${clientID}`);
    });
    this.edgeSocket.addEventListener("message", (event) => this.handleEdgeMessage(String(event.data)));
    this.edgeSocket.addEventListener("close", () => {
      this.edgeOpen = false;
      if (this.edgeHeartbeatTimer) clearInterval(this.edgeHeartbeatTimer);
      this.edgeHeartbeatTimer = null;
    });
    this.edgeSocket.addEventListener("error", () => {});
    await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error("Insight Edge 网关连接超时")), 10000);
      this.edgeSocket.addEventListener("open", () => { clearTimeout(timer); resolve(); }, { once: true });
      this.edgeSocket.addEventListener("error", () => { clearTimeout(timer); reject(new Error("Insight Edge 网关连接失败")); }, { once: true });
    });
  }

  sendHeartbeat() {
    if (!this.edgeOpen || this.edgeSocket?.readyState !== WebSocket.OPEN) return;
    const state = this.lastState || {};
    const methods = state.methods || {};
    const payload = {
      hostname: os.hostname(), version: "insight-desktop", status: "running",
      page_path: state.pagePath || "", href: state.href || "",
      api_ready: !!state.apiReady, ws_clients: state.apiReady ? 1 : 0,
      ready_clients: state.apiReady ? 1 : 0,
      search_ready_clients: methods.finderSearch ? 1 : 0,
      feed_ready_clients: methods.finderUserPage ? 1 : 0,
      profile_ready_clients: methods.finderGetCommentDetail ? 1 : 0,
      finder_username: state.finderUsername || "",
      login_qr_code: state.loginQRCode || "",
      login_qr_code_url: state.loginQRCodeURL || "",
      supports_search: !!methods.finderSearch, supports_feed: !!methods.finderUserPage,
      supports_profile: !!methods.finderGetCommentDetail, methods,
    };
    this.edgeSocket.send(JSON.stringify({ id: `hb-${Date.now()}`, type: "heartbeat", client_id: clientID, payload, timestamp: Math.floor(Date.now() / 1000) }));
  }

  async handleEdgeMessage(raw) {
    let message;
    try { message = JSON.parse(raw); } catch { return; }
    if (message.type !== "command") return;
    let command;
    command = asObject(message.payload);
    if (command.action === "channel_navigate") {
      await this.openLogin().catch(() => {});
      this.respond(message.id, true, { accepted: true });
      return;
    }
    if (command.action === "channel_reload") {
      const page = await this.findChannelPage().catch(() => null);
      if (page) await page.command("Page.reload", { ignoreCache: false }).catch(() => {});
      this.respond(message.id, true, { accepted: true });
      return;
    }
    if (command.action !== "api_call") return;
    let call;
    call = asObject(command.data);
    const body = asObject(call.body);
    try {
      const data = await this.callPageAPI(call.key, body);
      this.respond(message.id, true, data);
    } catch (error) {
      this.respond(message.id, false, null, error instanceof Error ? error.message : String(error));
    }
  }

  async callPageAPI(key, body) {
    const page = await this.findChannelPage();
    if (!page) throw new Error("内置视频号助手页面尚未打开，请先点击登录");
    if (key === "key:channels:feed_list") {
      const username = String(body.username || "");
      if (!username) throw new Error("缺少视频号 ID");
      let pagePath = await page.evaluate("location.pathname").catch(() => "");
      if (!String(pagePath).endsWith("/platform/post/list")) {
        await page.navigate("https://channels.weixin.qq.com/platform/post/list");
        const until = Date.now() + 15000;
        while (Date.now() < until) {
          await sleep(250);
          pagePath = await page.evaluate("location.pathname").catch(() => "");
          if (String(pagePath).endsWith("/platform/post/list")) break;
        }
        if (!String(pagePath).endsWith("/platform/post/list")) {
          throw new Error("视频号作品页面加载超时");
        }
        await sleep(1200);
      }
      const source = `(async () => {
        const username = ${JSON.stringify(username)};
        const marker = ${JSON.stringify(String(body.next_marker || ""))};
        const query = new URLSearchParams({ _aid: crypto.randomUUID(), _rid: String(Date.now()), _pageUrl: location.href });
        const payload = { pageSize: 20, currentPage: 1, userpageType: 11, stickyOrder: true, timestamp: String(Date.now()), _log_finder_uin: '', _log_finder_id: '', rawKeyBuff: marker ? decodeURIComponent(marker) : '', pluginSessionId: null, scene: 7, reqScene: 7 };
        const response = await fetch('/micro/content/cgi-bin/mmfinderassistant-bin/post/post_list?' + query.toString(), { method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'include', body: JSON.stringify(payload) });
        if (!response.ok) throw new Error('视频号作品接口 HTTP ' + response.status);
        const root = await response.json();
        if (root.errCode && root.errCode !== 0) throw new Error(root.errMsg || '视频号作品接口返回错误');
        const data = root.data || {};
        const nickname = document.querySelector('.finder-nickname')?.textContent?.trim() || username;
        const items = Array.isArray(data.list) ? data.list.map(item => ({
          id: item.objectId || item.exportId || '', objectNonceId: item.objectNonce || '', createtime: item.createTime || 0,
          likeCount: item.likeCount || 0, commentCount: item.commentCount || 0, readCount: item.readCount || 0,
          forwardCount: item.forwardCount || 0, favCount: item.favCount || 0,
          contact: { username, nickname }, objectDesc: item.desc || {},
        })) : [];
        return { code: 0, message: 'success', data: { errCode: 0, errMsg: '', object: items, objectList: items, lastBuffer: data.continueFlag ? (data.lastBuff || '') : '', payload: { username, finderUsername: username } } };
      })()`;
      return page.evaluate(source);
    }
    const source = `(async () => {
      const body = ${JSON.stringify(body)};
      const wait = async () => { const end = Date.now() + 10000; while (Date.now() < end) { if (window.WXU?.API || window.WXU?.API2) return; await new Promise(r => setTimeout(r, 250)); } throw new Error('视频号助手页面 API 尚未就绪'); };
      await wait();
      const marker = body.next_marker ? decodeURIComponent(body.next_marker) : '';
      if (${JSON.stringify(key)} === 'key:channels:feed_list') return { ...(await window.WXU.API.finderUserPage({ username: body.username, finderUsername: window.__wx_username || '', lastBuffer: marker, needFansCount: 0, objectId: '0' })), payload: { username: body.username, lastBuffer: marker } };
      if (${JSON.stringify(key)} === 'key:channels:contact_list') return { ...(await window.WXU.API2.finderSearch({ query: body.keyword, scene: body.type == 3 ? 19 : 13, requestId: String(Date.now()), lastBuffer: marker, lastBuff: marker })), payload: body };
      if (${JSON.stringify(key)} === 'key:channels:feed_profile') return { ...(await window.WXU.API.finderGetCommentDetail({ needObject: 1, lastBuffer: '', scene: 146, direction: 2, identityScene: 2, pullScene: 6, objectid: String(body.objectId || body.object_id || '').split('_')[0], objectNonceId: body.nonceId || body.nonce_id || '' })), payload: body };
      if (${JSON.stringify(key)} === 'key:channels:fetch_feed_comment_list') return { ...(await window.WXU.API.finderGetCommentList({ objectId: body.object_id, objectNonceId: body.nonce_id, direction: 2, identityScene: 2, lastBuffer: marker })), payload: body };
      throw new Error('未匹配的 key: ' + ${JSON.stringify(key)});
    })()`;
    const result = await page.evaluate(source);
    return result;
  }

  respond(requestID, success, data, error = "") {
    if (!this.edgeOpen || this.edgeSocket?.readyState !== WebSocket.OPEN) return;
    const payload = { request_id: requestID, success, data: data == null ? {} : data, error };
    this.edgeSocket.send(JSON.stringify({ id: requestID, type: "response", client_id: clientID, payload, timestamp: Math.floor(Date.now() / 1000) }));
  }

  stop() {
    if (this.stateTimer) clearInterval(this.stateTimer);
    if (this.edgeHeartbeatTimer) clearInterval(this.edgeHeartbeatTimer);
    this.page?.close();
    try { this.edgeSocket?.close(); } catch {}
    if (this.edgeProcess && !this.edgeProcess.killed) this.edgeProcess.kill();
  }
}

function startBridge(bridge) {
  const server = http.createServer(async (request, response) => {
    response.setHeader("Access-Control-Allow-Origin", "*");
    response.setHeader("Cache-Control", "no-store");
    if (request.method === "OPTIONS") { response.writeHead(204); response.end(); return; }
    const pathname = new URL(request.url || "/", `http://${request.headers.host}`).pathname;
    if (pathname === "/state") {
      response.setHeader("Content-Type", "application/json; charset=utf-8");
      response.end(JSON.stringify(await bridge.refreshPageState().catch(() => bridge.lastState)));
      return;
    }
    if (pathname === "/login" && request.method === "POST") {
      await bridge.openLogin().catch(() => {});
      response.setHeader("Content-Type", "application/json; charset=utf-8");
      response.end(JSON.stringify({ ok: true, url: loginURL }));
      return;
    }
    response.writeHead(404); response.end("not found");
  });
  server.listen(bridgePort, "127.0.0.1", () => console.log(`Insight Desktop Bridge: http://127.0.0.1:${bridgePort}`));
  return server;
}

const bridge = new InsightDesktopBridge();
const httpServer = startBridge(bridge);
const shutdown = () => { httpServer.close(); bridge.stop(); process.exit(0); };
process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);

bridge.start().catch((error) => {
  console.error(error instanceof Error ? error.message : error);
  httpServer.close();
  process.exitCode = 1;
});
