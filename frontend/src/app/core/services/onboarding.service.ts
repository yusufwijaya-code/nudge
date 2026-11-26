import { Injectable, inject, signal } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { tap } from 'rxjs';
import { environment } from '../../../environments/environment';

/** One Jira board with the user's tickets in its active sprint (GET /api/onboarding/boards). */
export interface OnboardingBoard {
  boardId: number;
  name: string;
  projectKey: string;
  sprintName: string;
  ticketCount: number;
  /** The board's project has an Aturan Board. */
  registered: boolean;
}

/** The suggested form of a new board; lists can be null (nothing fit). */
export interface OnboardingDefaults {
  trackedStatuses: string[] | null;
  doneStatus: string;
  reviewStatus: string;
  storyPointFields: string[] | null;
  storyPointWriteField: string;
  storyPointWriteFieldSubtask: string;
}

export interface OnboardingField { id: string; name: string; storyPointLike?: boolean }

export interface OnboardingForm {
  board: OnboardingBoard;
  projectName: string;
  statuses: string[] | null;
  numberFields: OnboardingField[] | null;
  defaults: OnboardingDefaults;
  /** The existing rule the suggestion was copied from ('' = none fit). */
  templateProject: string;
}

export interface OnboardingInput {
  trackedStatuses: string[];
  doneStatus: string;
  reviewStatus: string | null;
  storyPointFields: string[];
  storyPointWriteField: string;
  storyPointWriteFieldSubtask: string;
}

export interface OnboardingResult { projectKey: string; synced: boolean; added: number; updated: number }

export type OnboardingFieldName = 'trackedStatuses' | 'doneStatus' | 'reviewStatus' | 'storyPointFields' | 'storyPointWriteField' | 'storyPointWriteFieldSubtask';

/**
 * Step 2 of joining Nudge: which of the user's boards are not registered yet and
 * the setup of one. `unregistered` feeds the navbar picker and the redirect to
 * /onboarding for a user whose boards are all new.
 */
@Injectable({ providedIn: 'root' })
export class OnboardingService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/onboarding`;

  readonly unregistered = signal<OnboardingBoard[]>([]);
  /** The boards were asked once (the redirect waits for it). */
  readonly loaded = signal(false);

  boards(fresh = false) {
    const params = new HttpParams({ fromObject: fresh ? { fresh: '1' } : {} });
    return this.http.get<{ boards: OnboardingBoard[] | null }>(`${this.base}/boards`, { params });
  }

  /** Loads the picker's "Belum diatur" list; a failure leaves it empty. */
  refresh(fresh = false): void {
    this.boards(fresh).subscribe({
      next: (res) => {
        this.unregistered.set((res.boards ?? []).filter((b) => !b.registered));
        this.loaded.set(true);
      },
      error: () => this.loaded.set(true),
    });
  }

  form(boardId: number) { return this.http.get<OnboardingForm>(`${this.base}/boards/${boardId}/form`); }

  setup(boardId: number, body: OnboardingInput) {
    return this.http.post<OnboardingResult>(`${this.base}/boards/${boardId}/setup`, body).pipe(
      tap(() => this.unregistered.update((list) => list.filter((b) => b.boardId !== boardId))),
    );
  }
}
