#!/usr/bin/env node
// Downloads the correct ccpm binary for the current platform during npm install

const { execFileSync } = require("child_process");
const crypto = require("crypto");
const fs = require("fs");
const path = require("path");
const https = require("https");

const REPO = "nitin-1926/claude-code-profile-manager";
const BINARY = "ccpm";

// Pin the download to the version of this exact npm package. Querying
// releases/latest would let a freshly cut release ship a binary that doesn't
// match the installed package version (and could be raced by an attacker who
// can publish a release). require()ing package.json keeps the two in lockstep.
const VERSION = require("./package.json").version;

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

// Fetch a small text resource (the checksums manifest), following redirects.
function fetchText(url) {
  return new Promise((resolve, reject) => {
    const follow = (url) => {
      https.get(url, { headers: { "User-Agent": "ccpm-npm" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          follow(res.headers.location);
          return;
        }
        if (res.statusCode !== 200) {
          reject(new Error(`HTTP ${res.statusCode} for ${url}`));
          return;
        }
        let data = "";
        res.on("data", (chunk) => (data += chunk));
        res.on("end", () => resolve(data));
        res.on("error", reject);
      }).on("error", reject);
    };
    follow(url);
  });
}

function sha256OfFile(filePath) {
  const hash = crypto.createHash("sha256");
  hash.update(fs.readFileSync(filePath));
  return hash.digest("hex");
}

// Parse goreleaser's checksums.txt ("<sha256>  <filename>" per line) and return
// the expected hash for archiveName, or null if it isn't listed.
function expectedChecksum(manifest, archiveName) {
  for (const line of manifest.split("\n")) {
    const m = line.match(/^([0-9a-fA-F]{64})\s+(\S+)\s*$/);
    if (m && m[2] === archiveName) return m[1].toLowerCase();
  }
  return null;
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

function warnIfUnverifiedPlatform(os) {
  if (os === "darwin") return;
  const yellow = "\x1b[33m";
  const reset = "\x1b[0m";
  process.stderr.write(
    `${yellow}ccpm: ${os} support is experimental.${reset} ` +
      `OAuth set-default / auth backup / status are verified only on macOS today. ` +
      `API-key profiles work everywhere. Track Linux/Windows readiness at ` +
      `https://github.com/${REPO}/issues\n`,
  );
}

async function main() {
  const { os, arch } = getPlatform();
  warnIfUnverifiedPlatform(os);
  const version = VERSION;

  const ext = os === "windows" ? "zip" : "tar.gz";
  const archiveName = `${BINARY}_${os}_${arch}.${ext}`;
  const baseUrl = `https://github.com/${REPO}/releases/download/v${version}`;
  const url = `${baseUrl}/${archiveName}`;
  const checksumsUrl = `${baseUrl}/checksums.txt`;

  console.log(`Installing ccpm v${version} for ${os}/${arch}...`);

  const binDir = path.join(__dirname, "bin");
  fs.mkdirSync(binDir, { recursive: true });

  // Extract into a temp subdirectory so the archive's `ccpm` binary doesn't
  // overwrite the JS shim that npm uses to create the global `ccpm` command.
  const tmpDir = path.join(binDir, ".extract");
  if (fs.existsSync(tmpDir)) fs.rmSync(tmpDir, { recursive: true, force: true });
  fs.mkdirSync(tmpDir, { recursive: true });

  const archivePath = path.join(tmpDir, `archive.${ext}`);
  try {
    await download(url, archivePath);
  } catch (err) {
    throw new Error(
      `failed to download ${url}: ${err.message}\n` +
        `  Is there a published release for v${version}? See https://github.com/${REPO}/releases`,
    );
  }

  // Verify the archive against the SHA-256 published in the SAME release's
  // checksums.txt. Fail closed: a missing manifest, a missing line, or a
  // mismatch deletes the partial download and aborts — we never install an
  // unverified binary that would run with the user's privileges.
  let manifest;
  try {
    manifest = await fetchText(checksumsUrl);
  } catch (err) {
    fs.rmSync(tmpDir, { recursive: true, force: true });
    throw new Error(
      `failed to download ${checksumsUrl}: ${err.message}\n` +
        `  Refusing to install without a checksum to verify against.`,
    );
  }

  console.log("Verifying SHA-256...");
  const expected = expectedChecksum(manifest, archiveName);
  if (!expected) {
    fs.rmSync(tmpDir, { recursive: true, force: true });
    throw new Error(
      `${archiveName} not listed in checksums.txt — possible tampering or missing release asset.`,
    );
  }
  const actual = sha256OfFile(archivePath);
  if (expected !== actual) {
    fs.rmSync(tmpDir, { recursive: true, force: true });
    throw new Error(
      `checksum mismatch for ${archiveName}.\n` +
        `  expected: ${expected}\n` +
        `  actual:   ${actual}\n` +
        `  Refusing to install a tampered binary.`,
    );
  }
  console.log(`  sha256 ok (${expected})`);

  // Extract via argv-array exec (no shell interpolation of paths). The
  // archive content is already SHA-256-verified against the cosign-signed
  // checksums manifest above, so residual zip-slip risk is accepted rather
  // than adding a JS extractor dependency (itself supply-chain surface).
  if (ext === "zip") {
    execFileSync("unzip", ["-o", "-q", archivePath, "-d", tmpDir], { stdio: "inherit" });
  } else {
    execFileSync("tar", ["-xzf", archivePath, "-C", tmpDir], { stdio: "inherit" });
  }

  // Move the native binary to bin/ccpm-native (or ccpm-native.exe on Windows).
  // The JS shim at bin/ccpm execs this at runtime.
  const extractedName = os === "windows" ? `${BINARY}.exe` : BINARY;
  const nativeName = os === "windows" ? "ccpm-native.exe" : "ccpm-native";
  const extractedPath = path.join(tmpDir, extractedName);
  const nativePath = path.join(binDir, nativeName);

  if (!fs.existsSync(extractedPath)) {
    throw new Error(`extracted binary not found at ${extractedPath}`);
  }
  // Belt-and-braces: the file we are about to install must really live inside
  // the temp dir (no symlink escape from a hostile archive).
  const realExtracted = fs.realpathSync(extractedPath);
  const realTmp = fs.realpathSync(tmpDir);
  if (realExtracted !== path.join(realTmp, extractedName)) {
    fs.rmSync(tmpDir, { recursive: true, force: true });
    throw new Error(`extracted binary resolves outside the staging dir: ${realExtracted}`);
  }
  const extractedStat = fs.lstatSync(extractedPath);
  if (!extractedStat.isFile()) {
    fs.rmSync(tmpDir, { recursive: true, force: true });
    throw new Error(`extracted binary is not a regular file`);
  }

  if (fs.existsSync(nativePath)) fs.unlinkSync(nativePath);
  fs.renameSync(extractedPath, nativePath);

  if (os !== "windows") {
    fs.chmodSync(nativePath, 0o755);
  }

  // Clean up temp dir
  fs.rmSync(tmpDir, { recursive: true, force: true });

  console.log(`ccpm v${version} installed successfully!`);
}

main().catch((err) => {
  console.error("Failed to install ccpm:", err.message);
  console.error("You can install manually: https://github.com/" + REPO + "/releases");
  process.exit(1);
});
