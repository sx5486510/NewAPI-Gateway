#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { spawnSync } from 'node:child_process';

import {
  buildBillingSummary,
  extractAccessToken,
  extractUserId,
  formatQuotaReport,
  hasUsableQuota,
  jwtTierName,
} from './probe-grok-quota-lib.mjs';

const BASE_URL = 'https://cli-chat-proxy.grok.com/v1';
const CLIENT_VERSION = '0.2.102';
const CLIENT_IDENTIFIER = 'grok-shell';
const TOKEN_AUTH = 'xai-grok-cli';
const USER_AGENT = `grok-shell/${CLIENT_VERSION} (linux; x86_64)`;
const REQUEST_TIMEOUT_MS = 20000;

function restartWithEnvProxyIfNeeded() {
  if (process.env.NODE_USE_ENV_PROXY) return false;
  const hasProxy = ['HTTPS_PROXY', 'HTTP_PROXY', 'ALL_PROXY', 'https_proxy', 'http_proxy', 'all_proxy']
    .some((key) => String(process.env[key] || '').trim());
  if (!hasProxy) return false;
  const result = spawnSync(process.execPath, process.argv.slice(1), {
    env: { ...process.env, NODE_USE_ENV_PROXY: '1' },
    stdio: 'inherit',
  });
  if (result.error) throw result.error;
  process.exit(result.status ?? 1);
}

function usage(message) {
  if (message) console.error(message);
  console.error(
    'Usage: node scripts/probe-grok-quota.mjs <path-to-grok-credential.json>'
  );
  process.exit(2);
}

function readCredential(filePath) {
  let raw;
  try {
    raw = fs.readFileSync(filePath, 'utf8');
  } catch (error) {
    throw new Error(`Unable to read credential file: ${error.message}`);
  }
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      throw new Error('credential root must be a JSON object');
    }
    return parsed;
  } catch (error) {
    throw new Error(`Invalid credential JSON: ${error.message}`);
  }
}

function buildHeaders(accessToken, userId) {
  const headers = {
    Authorization: `Bearer ${accessToken}`,
    'X-XAI-Token-Auth': TOKEN_AUTH,
    'x-grok-client-version': CLIENT_VERSION,
    'x-grok-client-identifier': CLIENT_IDENTIFIER,
    'x-grok-client-mode': 'headless',
    Accept: 'application/json',
    'User-Agent': USER_AGENT,
  };
  if (userId) headers['x-userid'] = userId;
  return headers;
}

async function fetchJson(url, headers) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
  try {
    const response = await fetch(url, {
      method: 'GET',
      headers,
      signal: controller.signal,
    });
    const text = await response.text();
    let body = null;
    if (text.trim()) {
      try {
        body = JSON.parse(text);
      } catch {
        body = null;
      }
    }
    return {
      ok: response.ok,
      status: response.status,
      body,
      error: response.ok
        ? null
        : `HTTP ${response.status}${
            body && typeof body === 'object' && body.error
              ? `: ${typeof body.error === 'string' ? body.error : JSON.stringify(body.error)}`
              : ''
          }`,
    };
  } catch (error) {
    const message =
      error?.name === 'AbortError'
        ? `Request timed out after ${REQUEST_TIMEOUT_MS}ms`
        : error.message;
    return {
      ok: false,
      status: null,
      body: null,
      error: message,
    };
  } finally {
    clearTimeout(timer);
  }
}

async function main() {
  restartWithEnvProxyIfNeeded();
  const credentialPath = process.argv[2];
  if (!credentialPath || credentialPath === '-h' || credentialPath === '--help') {
    usage();
  }

  const absolutePath = path.resolve(credentialPath);
  const credential = readCredential(absolutePath);
  const accessToken = extractAccessToken(credential);
  if (!accessToken) {
    throw new Error('Credential does not contain an access token');
  }
  const userId = extractUserId(credential);
  const headers = buildHeaders(accessToken, userId);

  const endpoints = {
    credits: `${BASE_URL}/billing?format=credits`,
    billing: `${BASE_URL}/billing`,
    user: `${BASE_URL}/user?include=subscription`,
  };

  const [credits, billing, user] = await Promise.all([
    fetchJson(endpoints.credits, headers),
    fetchJson(endpoints.billing, headers),
    fetchJson(endpoints.user, headers),
  ]);

  let subscriptionTier = null;
  if (user.ok && user.body && typeof user.body === 'object') {
    subscriptionTier =
      (typeof user.body.subscriptionTier === 'string' &&
        user.body.subscriptionTier) ||
      (typeof user.body.subscription_tier === 'string' &&
        user.body.subscription_tier) ||
      (user.body.user &&
        typeof user.body.user === 'object' &&
        (user.body.user.subscriptionTier || user.body.user.subscription_tier)) ||
      null;
    if (typeof subscriptionTier === 'string') {
      subscriptionTier = subscriptionTier.trim() || null;
    } else {
      subscriptionTier = null;
    }
  }

  const summary = buildBillingSummary({
    credits: credits.ok ? credits.body : null,
    billing: billing.ok ? billing.body : null,
    subscriptionTier,
    jwtTier: jwtTierName(accessToken),
    sources: {
      credits: credits.status,
      billing: billing.status,
      user: user.status,
    },
  });

  console.log(`credential: ${path.basename(absolutePath)}`);
  if (userId) console.log(`user_id: ${userId}`);
  console.log(formatQuotaReport(summary));

  const authFailed = [credits, billing].every(
    (item) => item.status === 401 || item.status === 403
  );
  if (authFailed) {
    console.error(
      'Both billing endpoints returned 401/403. Access token is expired or unauthorized; refresh the credential and retry.'
    );
    process.exit(1);
  }

  if (!credits.ok && !billing.ok) {
    console.error(
      `Both billing endpoints failed. credits=${credits.error}; billing=${billing.error}`
    );
    process.exit(1);
  }

  if (!hasUsableQuota(summary)) {
    console.error('Billing responses returned no usable quota fields.');
    process.exit(1);
  }

  if (!user.ok) {
    console.error(
      `Warning: subscription lookup failed (${user.error ?? 'unknown error'}); plan may fall back to billing/JWT fields.`
    );
  }
}

main().catch((error) => {
  console.error(error.message || String(error));
  process.exit(1);
});
