import { chromium } from '@playwright/test';
import assert from 'node:assert/strict';
const browser=await chromium.launch({executablePath:'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true});
try {
 const page=await browser.newPage();await page.goto('http://192.168.1.16:5252/login');await page.locator('#passkey').click();await page.getByText(/当前页面使用不安全的 HTTP 地址/).waitFor();console.log('PASS: HTTP LAN explains secure-context restriction');
 const secure=await browser.newPage();await secure.addInitScript(()=>{navigator.credentials.get=async()=>{window.passkeyCalled=true;throw new DOMException('test cancellation','NotAllowedError')}});await secure.goto('https://id.duoyu.link/login');await secure.locator('#passkey').click();await secure.waitForFunction(()=>window.passkeyCalled===true);await secure.getByText('通行密钥操作已取消或超时。',{exact:true}).waitFor();console.log('PASS: HTTPS PC Chrome reaches navigator.credentials.get and handles cancellation');
 await page.close();await secure.close();
} finally {await browser.close()}
