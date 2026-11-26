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

describe('DateRangePickerComponent (range, two months side by side)', () => {
  let fixture: ComponentFixture<DateRangePickerComponent>;
  let picked: DateRange[];
  const el = () => fixture.nativeElement as HTMLElement;
  const q = <T extends HTMLElement>(selector: string) => el().querySelector(selector) as T | null;
  const panels = () => Array.from(el().querySelectorAll('[data-panel]')) as HTMLElement[];
  const titles = () => panels().map((p) => p.querySelector('.ui-date-panel-title')!.textContent!.trim());
  const day = (date: string) => q<HTMLButtonElement>(`[data-day="${date}"]`)!;

  beforeEach(async () => {
    await TestBed.configureTestingModule({ imports: [DateRangePickerComponent] }).compileComponents();
    fixture = TestBed.createComponent(DateRangePickerComponent);
    picked = [];
    fixture.componentInstance.rangeChange.subscribe((r) => picked.push(r));
    fixture.componentRef.setInput('months', 2);
    fixture.componentRef.setInput('from', '2026-10-28');
    fixture.componentRef.setInput('to', '2026-11-26');
    fixture.detectChanges();
    (q('.ui-date-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();
  });

  it('opens with two months, the start month first, so the end of the range is in view', () => {
    expect(panels().length).toBe(2);
    expect(titles()).toEqual(['Oktober 2026', 'November 2026']);
    expect(q('.ui-date-popup')!.classList).toContain('dual');
    expect(day('2026-10-28').classList).toContain('selected');
    expect(day('2026-11-26').classList).toContain('selected');
    expect(day('2026-11-05').classList).toContain('in-range');
  });

  it('shows each date once: the days of the neighbouring month are left blank, not repeated', () => {
    const all = Array.from(el().querySelectorAll('[data-day]')).map((b) => b.getAttribute('data-day'));
    expect(new Set(all).size).toBe(all.length);
    expect(all.length).toBe(31 + 30);
    expect(el().querySelectorAll('.ui-date-cell.outside').length).toBe(0);
  });

  it('moves both months with the arrows, and by a year with yearNav', () => {
    (q('[data-next]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(titles()).toEqual(['November 2026', 'Desember 2026']);
    (q('[data-prev]') as HTMLButtonElement).click();
    (q('[data-prev]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(titles()).toEqual(['September 2026', 'Oktober 2026']);
    expect(q('[data-prev-year]')).toBeNull();
  });

  it('picks a range that spans the two months with two clicks', () => {
    day('2026-10-15').click();
    fixture.detectChanges();
    expect(q('.ui-date-step')!.textContent).toContain('selesai');
    day('2026-11-20').click();
    fixture.detectChanges();
    expect(picked).toEqual([{ from: '2026-10-15', to: '2026-11-20' }]);
    expect(q('.ui-date-popup')).toBeNull();
  });

  it('highlights the range while hovering across the two months', () => {
    day('2026-10-20').click();
    fixture.detectChanges();
    day('2026-11-03').dispatchEvent(new Event('mouseenter'));
    fixture.detectChanges();
    expect(day('2026-10-31').classList).toContain('in-range');
    expect(day('2026-11-02').classList).toContain('in-range');
    expect(day('2026-11-04').classList).not.toContain('in-range');
  });

  it('does not let days after max be picked in either month', () => {
    fixture.componentRef.setInput('max', '2026-11-10');
    fixture.detectChanges();
    expect(day('2026-11-10').disabled).toBeFalse();
    expect(day('2026-11-11').disabled).toBeTrue();
    expect(day('2026-10-31').disabled).toBeFalse();
  });

  it('stays a single month unless asked', () => {
    const f = TestBed.createComponent(DateRangePickerComponent);
    f.componentRef.setInput('from', '2026-10-28');
    f.detectChanges();
    (f.nativeElement.querySelector('.ui-date-trigger') as HTMLButtonElement).click();
    f.detectChanges();
    expect(f.nativeElement.querySelectorAll('[data-panel]').length).toBe(1);
    expect(f.nativeElement.querySelector('.ui-date-popup').classList).not.toContain('dual');
    expect(f.nativeElement.querySelector('.ui-date-title').textContent.trim()).toBe('Oktober 2026');
  });
});
