import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

/** The "geser otomatis ke selesai" switch of the signed-in user (Pengaturan › Umum). */
export interface AutoDoneStatus {
  /** The user's choice. */
  enabled: boolean;
  /** A personal Jira API key is saved (the feature needs it). */
  hasCredential: boolean;
  /** Enabled and a key exists: the feature is really working. */
  active: boolean;
}

@Injectable({ providedIn: 'root' })
export class AutoDoneService {
  private http = inject(HttpClient);
  private url = `${environment.apiBasePath}/api/dashboard/settings/auto-done`;

  get(): Observable<AutoDoneStatus> {
    return this.http.get<AutoDoneStatus>(this.url);
  }

  set(enabled: boolean): Observable<AutoDoneStatus> {
    return this.http.put<AutoDoneStatus>(this.url, { enabled });
  }
}
