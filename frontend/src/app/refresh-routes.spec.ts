import { TestBed } from '@angular/core/testing';
import { provideRouter, Router } from '@angular/router';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Component } from '@angular/core';
import { routes } from './app.routes';

@Component({ standalone: true, template: '' })
class Stub {}

/** Every guarded route as the real route table declares it, with the lazy/eager pages stubbed out. */
function stubbed(list: typeof routes): typeof routes {
  return list.map((r) => {
    const { component, loadComponent, ...rest } = r as any;
    void component; void loadComponent;
    return { ...rest, ...(r.redirectTo === undefined ? { component: Stub } : {}), ...(r.children ? { children: stubbed(r.children) } : {}) };
  });
}

const DASHBOARD = ['/dashboard', '/dashboard/board', '/dashboard/leave', '/dashboard/calendar', '/dashboard/settings'];
const ADMIN = ['/admin', '/admin/users', '/admin/users/8', '/admin/broadcast', '/admin/chat-logs', '/admin/jira-account', '/admin/tracking-rules', '/admin/tracking-rules/new', '/admin/tracking-rules/CORE'];

/**
 * A page refresh = a brand-new app (session not loaded yet) opened straight on
 * a deep link. It must stay on that page, not fall back to the main dashboard.
 */
describe('refreshing on a page', () => {
  function refreshOn(url: string, me: { status: number; body?: unknown }): Promise<string> {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [provideRouter(stubbed(routes)), provideHttpClient(), provideHttpClientTesting()] });
    const http = TestBed.inject(HttpTestingController);
    const router = TestBed.inject(Router);
    const done = router.navigateByUrl(url).then(() => router.url);
    // Every guard that needs the session asks /api/auth/me.
    queueMicrotask(() => {
      for (const req of http.match((r) => r.url.endsWith('/api/auth/me'))) {
        if (me.status === 200) req.flush(me.body as object);
        else req.flush({}, { status: me.status, statusText: 'x' });
      }
    });
    return done.then(async (u) => {
      await Promise.resolve();
      return u;
    });
  }

  for (const url of DASHBOARD) {
    it(`stays on ${url} for a signed-in user`, async () => {
      expect(await refreshOn(url, { status: 200, body: { email: 'a@esb.co.id', isAdmin: false } })).toBe(url);
    });
  }
  for (const url of ADMIN) {
    it(`stays on ${url} for an admin`, async () => {
      expect(await refreshOn(url, { status: 200, body: { email: 'a@esb.co.id', isAdmin: true } })).toBe(url);
    });
    it(`sends a non-admin from ${url} to the dashboard`, async () => {
      expect(await refreshOn(url, { status: 200, body: { email: 'a@esb.co.id', isAdmin: false } })).toBe('/dashboard');
    });
  }
  it('sends a signed-out visitor to the login page', async () => {
    expect(await refreshOn('/admin/users', { status: 401 })).toBe('/');
    expect(await refreshOn('/dashboard/board', { status: 401 })).toBe('/');
  });
});
