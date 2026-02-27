#!/usr/bin/env node
/**
 * Decrypt Chrome v10/v11 cookies on Linux (KDE/KWallet).
 *
 * Both v10 and v11 use AES-128-CBC with IV = 16 spaces.
 * Key = PBKDF2-HMAC-SHA1(password, "saltysalt", 1 iteration, 16 bytes)
 *
 * v10 password: "peanuts" (hardcoded)
 * v11 password: stored in KWallet under "Chrome Keys / Chrome Safe Storage"
 *
 * v11 format (Chrome 142+): 'v11' (3 bytes) + 32-byte header + AES-CBC(plaintext)
 *   The first 32 bytes of ciphertext are a per-scope header. After CBC decryption,
 *   strip the first 32 bytes of plaintext to get the actual cookie value.
 *
 * Output: a Netscape cookie jar file suitable for curl -b.
 */

import { createDecipheriv, pbkdf2Sync } from 'node:crypto';
import { writeFileSync } from 'node:fs';
import Database from 'better-sqlite3';
import { existsSync } from 'node:fs';

// --- Config ---
const CHROME_PASSWORD = process.argv[2] || 'N77BMq1CCPAZXwUbNtGlZQ==';
const COOKIES_DB = process.argv[3] ||
  `${process.env.HOME}/.config/google-chrome/Default/Cookies`;
const OUTPUT_FILE = process.argv[4] || 'google-cookies.txt';

// Domains we need cookies from
const DOMAINS = [
  '.google.com',
  '.google.it',
  'accounts.google.com',
  'photos.google.com',
  'photos.fife.usercontent.google.com',
];

// --- Derive keys (both 16 bytes, AES-128-CBC) ---
const IV = Buffer.alloc(16, ' '); // 16 spaces

// v10: hardcoded password "peanuts"
const keyV10 = pbkdf2Sync('peanuts', 'saltysalt', 1, 16, 'sha1');

// v11: password from KWallet (the string as-is)
const keyV11 = pbkdf2Sync(CHROME_PASSWORD, 'saltysalt', 1, 16, 'sha1');

function decryptValue(encBuf) {
  if (!encBuf || encBuf.length === 0) return '';

  const prefix = encBuf.slice(0, 3).toString('ascii');

  let key;
  let skipBytes = 0;
  if (prefix === 'v11') {
    key = keyV11;
    skipBytes = 32; // v11 has 32-byte header in ciphertext
  } else if (prefix === 'v10') {
    key = keyV10;
    skipBytes = 0;
  } else {
    // Not encrypted
    return encBuf.toString('utf8');
  }

  const ciphertext = encBuf.slice(3);

  try {
    const decipher = createDecipheriv('aes-128-cbc', key, IV);
    decipher.setAutoPadding(false);
    let decrypted = decipher.update(ciphertext);
    decrypted = Buffer.concat([decrypted, decipher.final()]);

    // Remove PKCS7 padding manually
    const padLen = decrypted[decrypted.length - 1];
    if (padLen > 0 && padLen <= 16) {
      decrypted = decrypted.slice(0, decrypted.length - padLen);
    }

    // Skip header bytes
    if (skipBytes > 0 && decrypted.length > skipBytes) {
      decrypted = decrypted.slice(skipBytes);
    }

    return decrypted.toString('utf8');
  } catch (e) {
    // On failure for v11, try the empty-password fallback
    if (prefix === 'v11') {
      const emptyKey = pbkdf2Sync('', 'saltysalt', 1, 16, 'sha1');
      try {
        const dec2 = createDecipheriv('aes-128-cbc', emptyKey, IV);
        dec2.setAutoPadding(false);
        let r = dec2.update(ciphertext);
        r = Buffer.concat([r, dec2.final()]);
        const pad = r[r.length - 1];
        if (pad > 0 && pad <= 16) r = r.slice(0, r.length - pad);
        if (r.length > skipBytes) r = r.slice(skipBytes);
        return r.toString('utf8');
      } catch (e2) { /* fall through */ }
    }
    console.error(`  [warn] ${prefix} decrypt failed: ${e.message}`);
    return '';
  }
}

// Chrome timestamp epoch: Jan 1, 1601 -> JS epoch: Jan 1, 1970
const CHROME_EPOCH_OFFSET = 11644473600n * 1000000n;
function chromeTimeToUnix(chromeUs) {
  if (!chromeUs || chromeUs === 0n) return 0;
  return Number((BigInt(chromeUs) - CHROME_EPOCH_OFFSET) / 1000000n);
}

function main() {
  if (!existsSync(COOKIES_DB)) {
    console.error(`Cookies DB not found: ${COOKIES_DB}`);
    process.exit(1);
  }

  const db = new Database(COOKIES_DB, { readonly: true });

  const placeholders = DOMAINS.map(() => '?').join(',');
  const rows = db.prepare(
    `SELECT host_key, name, encrypted_value, path, expires_utc,
            is_secure, is_httponly
     FROM cookies
     WHERE host_key IN (${placeholders})
     ORDER BY host_key, name`
  ).all(...DOMAINS);

  console.error(`Found ${rows.length} cookies for Google domains`);

  const lines = ['# Netscape HTTP Cookie File', '# Decrypted from Chrome', ''];

  let decrypted = 0;
  let failed = 0;

  for (const row of rows) {
    const value = decryptValue(row.encrypted_value);
    if (!value) {
      failed++;
      continue;
    }
    decrypted++;

    const expiresUnix = chromeTimeToUnix(BigInt(row.expires_utc));
    const httpOnly = row.is_httponly ? '#HttpOnly_' : '';
    const domain = row.host_key;
    const domainFlag = domain.startsWith('.') ? 'TRUE' : 'FALSE';
    const secureFl = row.is_secure ? 'TRUE' : 'FALSE';

    lines.push(
      `${httpOnly}${domain}\t${domainFlag}\t${row.path}\t${secureFl}\t${expiresUnix}\t${row.name}\t${value}`
    );
  }

  db.close();

  writeFileSync(OUTPUT_FILE, lines.join('\n') + '\n');

  console.error(`\nDecrypted: ${decrypted}, Failed: ${failed}`);
  console.error(`Cookie jar written to: ${OUTPUT_FILE}`);

  // Sanity check
  if (decrypted > 0) {
    console.error('\nSample cookies:');
    const db2 = new Database(COOKIES_DB, { readonly: true });
    const sample = db2.prepare(
      `SELECT host_key, name, encrypted_value FROM cookies
       WHERE host_key IN (${placeholders})
       AND name IN ('SID', 'OSID', 'COMPASS', '__Secure-OSID', 'NID')
       LIMIT 6`
    ).all(...DOMAINS);
    for (const row of sample) {
      const v = decryptValue(row.encrypted_value);
      if (v) console.error(`  ${row.host_key} :: ${row.name} = ${v.substring(0, 50)}...`);
    }
    db2.close();
  }
}

main();
