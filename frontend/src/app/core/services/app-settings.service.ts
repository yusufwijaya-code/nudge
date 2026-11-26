import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

/** Application-wide values an admin can change (Admin › Pengaturan Aplikasi). */
export interface AppSettings {
  /** Deviation a ticket / sprint may have, in percent, before it counts as over. */
  deviationCapPercent: number;
  /** What applies until an admin sets one. */
  defaultPercent: number;
  updatedBy?: string;
  updatedDate?: string;
}

export const MIN_DEVIATION = 1;
export const MAX_DEVIATION = 100;

@Injectable({ providedIn: 'root' })
export class AppSettingsService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard/admin/app-settings`;

  get(): Observable<AppSettings> {
    return this.http.get<AppSettings>(this.base);
  }

  saveDeviation(percent: number): Observable<AppSettings> {
    return this.http.put<AppSettings>(`${this.base}/deviation`, { deviationCapPercent: percent });
  }
}
