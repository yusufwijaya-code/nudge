import { TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { MyJiraTokenService } from './my-jira-token.service';
import { BoardService } from './board.service';

describe('MyJiraTokenService', () => {
  let board: jasmine.SpyObj<BoardService>;
  let service: MyJiraTokenService;

  beforeEach(() => {
    board = jasmine.createSpyObj('BoardService', ['getSettings']);
    TestBed.configureTestingModule({ providers: [{ provide: BoardService, useValue: board }] });
    service = TestBed.inject(MyJiraTokenService);
  });

  it('knows whether a token is saved and when it expires', () => {
    board.getSettings.and.returnValue(of({ credential: { hasCredential: true, maskedPreview: '••••ab3f', expiresAt: '2026-11-30' } } as any));
    service.refresh();
    expect(service.hasCredential()).toBeTrue();
    expect(service.expiresAt()).toBe('2026-11-30');
  });

  it('has nothing to warn about without a saved token or a date', () => {
    board.getSettings.and.returnValue(of({ credential: { hasCredential: false, maskedPreview: '' } } as any));
    service.refresh();
    expect(service.hasCredential()).toBeFalse();
    expect(service.expiresAt()).toBe('');
  });

  it('keeps what it knew when the settings cannot be loaded', () => {
    board.getSettings.and.returnValue(of({ credential: { hasCredential: true, maskedPreview: 'x', expiresAt: '2026-11-30' } } as any));
    service.refresh();
    board.getSettings.and.returnValue(throwError(() => ({ status: 500 })));
    service.refresh();
    expect(service.expiresAt()).toBe('2026-11-30');
  });
});
