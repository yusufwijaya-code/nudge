import { ComponentFixture, TestBed } from '@angular/core/testing';
import { DateRange, DateRangePickerComponent } from './date-range-picker.component';

describe('DateRangePickerComponent (single date, as used for "Berlaku sampai")', () => {
  let fixture: ComponentFixture<DateRangePickerComponent>;
  let picked: DateRange[];
  const el = () => fixture.nativeElement as HTMLElement;
  const q = <T extends HTMLElement>(selector: string) => el().querySelector(selector) as T | null;
  const title = () => q('.ui-date-title')?.textContent?.trim();

  beforeEach(async () => {
    await TestBed.configureTestingModule({ imports: [DateRangePickerComponent] }).compileComponents();
    fixture = TestBed.createComponent(DateRangePickerComponent);
    picked = [];
    fixture.componentInstance.rangeChange.subscribe((r) => picked.push(r));
    fixture.componentRef.setInput('single', true);
  });

  const open = (from = '2026-11-25') => {
    fixture.componentRef.setInput('from', from);
    fixture.detectChanges();
    (q('.ui-date-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();
  };

  it('shows the picked day on the trigger and the placeholder when none', () => {
    fixture.componentRef.setInput('placeholder', 'Pilih tanggal');
    fixture.detectChanges();
    expect(q('.ui-select-value')?.textContent).toContain('Pilih tanggal');
    fixture.componentRef.setInput('from', '2027-10-26');
    fixture.detectChanges();
    expect(q('.ui-select-value')?.textContent).toContain('26 Okt 2027');
  });

  it('picks one day with one click and closes', () => {
    open();
    (q('[data-day="2026-11-30"]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(picked).toEqual([{ from: '2026-11-30', to: '' }]);
    expect(q('.ui-date-popup')).toBeNull();
  });

  it('has no jump-a-year buttons unless asked for', () => {
    open();
    expect(q('[data-prev-year]')).toBeNull();
    expect(q('[data-next-year]')).toBeNull();
  });

  it('jumps a year at a time with yearNav, for a token that expires about a year away', () => {
    fixture.componentRef.setInput('yearNav', true);
    open('2026-11-25');
    expect(title()).toBe('November 2026');
    q('[data-next-year]')!.click();
    fixture.detectChanges();
    expect(title()).toBe('November 2027');
    q('[data-prev-year]')!.click();
    q('[data-prev-year]')!.click();
    fixture.detectChanges();
    expect(title()).toBe('November 2025');
    q('[data-next]')!.click();
    fixture.detectChanges();
    expect(title()).toBe('Desember 2025');
  });

  it('cannot be opened when disabled', () => {
    fixture.componentRef.setInput('disabled', true);
    fixture.componentRef.setInput('from', '');
    fixture.detectChanges();
    const trigger = q<HTMLButtonElement>('.ui-date-trigger')!;
    expect(trigger.disabled).toBeTrue();
    trigger.click();
    fixture.detectChanges();
    expect(q('.ui-date-popup')).toBeNull();
  });

  it('does not let days after max be picked', () => {
    fixture.componentRef.setInput('max', '2026-11-28');
    open();
    expect((q('[data-day="2026-11-28"]') as HTMLButtonElement).disabled).toBeFalse();
    expect((q('[data-day="2026-11-29"]') as HTMLButtonElement).disabled).toBeTrue();
  });

  it('forgets the day with Hapus', () => {
    open('2026-11-25');
    (q('[data-clear]') as HTMLButtonElement).click();
    expect(picked).toEqual([{ from: '', to: '' }]);
  });
});
