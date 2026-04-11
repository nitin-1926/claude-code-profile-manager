#!/usr/bin/env node
// Downloads the correct ccpm binary for the current platform during npm install

const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const https = require("https");

const REPO = "nitin-1926/claude-code-profile-manager";
const BINARY = "ccpm";

function getPlatform() {
  const platform = process.platform;
  const arch = process.arch;

  const osMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { x64: "amd64", arm64: "arm64" };

  const os = osMap[platform];
  const cpu = archMap[arch];

  if (!os || !cpu) {
    console.error(`Unsupported platform: ${platform}/${arch}`);
    process.exit(1);
  }

  return { os, arch: cpu };
}

function getLatestVersion() {
  return new Promise((resolve, reject) => {
    const url = `https://api.github.com/repos/${REPO}/releases/latest`;
    https.get(url, { headers: { "User-Agent": "ccpm-npm" } }, (res) => {
      let data = "";
      res.on("data", (chunk) => (data += chunk));
      res.on("end", () => {
        try {
          const json = JSON.parse(data);
          resolve(json.tag_name.replace(/^v/, ""));
        } catch {
          // Fallback to package.json version
          const pkg = require("./package.json");
          resolve(pkg.version);
        }
      });
      res.on("error", reject);
    });
  });
}

async function download(url, dest) {
  return new Promise((resolve, reject) => {
    const follow = (url) => {
      https.get(url, { headers: { "User-Agent": "ccpm-npm" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          follow(res.headers.location);
          return;
        }
        if (res.statusCode !== 200) {
          reject(new Error(`Download failed: HTTP ${res.statusCode}`));
          return;
        }
        const file = fs.createWriteStream(dest);
        res.pipe(file);
        file.on("finish", () => { file.close(); resolve(); });
        file.on("error", reject);
      });
    };
    follow(url);
  });
}

async function main() {
  const { os, arch } = getPlatform();
  const version = await getLatestVersion();

  const ext = os === "windows" ? "zip" : "tar.gz";
  const url = `https://github.com/${REPO}/releases/download/v${version}/${BINARY}_${os}_${arch}.${ext}`;

  console.log(`Installing ccpm v${version} for ${os}/${arch}...`);

  const binDir = path.join(__dirname, "bin");
  fs.mkdirSync(binDir, { recursive: true });

  const archivePath = path.join(binDir, `archive.${ext}`);
  await download(url, archivePath);

  // Extract
  if (ext === "zip") {
    execSync(`unzip -o -q "${archivePath}" -d "${binDir}"`, { stdio: "inherit" });
  } else {
    execSync(`tar -xzf "${archivePath}" -C "${binDir}"`, { stdio: "inherit" });
  }

  // Clean up archive
  fs.unlinkSync(archivePath);

  // Make executable
  const binaryPath = path.join(binDir, os === "windows" ? `${BINARY}.exe` : BINARY);
  if (os !== "windows") {
    fs.chmodSync(binaryPath, 0o755);
  }

  console.log(`ccpm v${version} installed successfully!`);
}

main().catch((err) => {
  console.error("Failed to install ccpm:", err.message);
  console.error("You can install manually: https://github.com/" + REPO + "/releases");
  process.exit(1);
});
