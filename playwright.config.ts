import {defineConfig} from '@playwright/test'
import {existsSync} from 'node:fs'
export default defineConfig({testDir:'./tests/e2e',fullyParallel:false,workers:1,timeout:30000,retries:0,reporter:'list',use:{baseURL:'http://127.0.0.1:4173',headless:true,launchOptions:{executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE||(existsSync('/usr/bin/chromium')?'/usr/bin/chromium':undefined),args:['--no-sandbox','--disable-dev-shm-usage']},trace:'retain-on-failure'},webServer:{command:'node tests/e2e/server.mjs',url:'http://127.0.0.1:4173/readyz',reuseExistingServer:false,timeout:60000}})
