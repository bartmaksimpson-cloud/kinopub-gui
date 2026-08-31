import { useEffect, useState } from "react";
import { ArrowLeft, CheckCircle2, FolderOpen, Stethoscope, TriangleAlert, Wrench } from "lucide-react";
import { api, type DoctorReport } from "../api";
import { useApp } from "../store";
import { useI18n } from "../i18n";
import { pushRoute } from "../router";
import { Field, Spinner, Toggle } from "../components/ui";
import { DirPicker } from "../components/DirPicker";

const KIND_LABEL: Record<string, string> = {
  MISSING: "Missing file",
  TRUNCATED: "Truncated",
  SIZE_MISMATCH: "Size mismatch",
  NO_PATH: "Incomplete record",
  ORPHAN_TMP: "Orphan .tmp",
};

// Statuses of a download that still has somewhere to go. A failed job counts:
// its Retry continues the partial segments it left behind. Mirrors
// isFinishedStatus in internal/gui/jobs.go.
const LIVE_STATUSES = ["queued", "resolving", "running", "paused", "failed"];

// pathsOverlap mirrors the server-side check in internal/gui/doctor.go: same
// folder, or one inside the other. Only an affordance — the server decides.
function pathsOverlap(a: string, b: string): boolean {
  const norm = (p: string) => p.replace(/[\\/]+$/, "").toLowerCase();
  const x = norm(a || "");
  const y = norm(b || "");
  if (!x || !y) return false;
  const inside = (child: string, parent: string) =>
    child.startsWith(parent + "/") || child.startsWith(parent + "\\");
  return x === y || inside(x, y) || inside(y, x);
}

export function DoctorPage() {
  const { settings, jobs, toast } = useApp();
  const { t } = useI18n();
  const [dir, setDir] = useState(settings.outputPath);

  // An unfinished download keeps its progress in exactly the temp files the
  // doctor deletes, and keeps rewriting the state file the doctor repairs — so
  // while one is using this folder, the destructive switches are off the table.
  // The server enforces this regardless; showing it here means the user isn't
  // left wondering why a toggle they flipped did nothing.
  const busyJobs = jobs.filter((j) => LIVE_STATUSES.includes(j.status) && pathsOverlap(dir, j.outputPath));
  const locked = busyJobs.length > 0;

  // Fill default folder once the SSE snapshot arrives (B6).
  // Does not overwrite a manual pick the user has already made.
  useEffect(() => {
    if (!dir) setDir(settings.outputPath);
  }, [settings.outputPath]);
  const [fix, setFix] = useState(false);
  const [cleanTmp, setCleanTmp] = useState(false);
  const [pickDir, setPickDir] = useState(false);
  const [running, setRunning] = useState(false);
  const [report, setReport] = useState<DoctorReport | null>(null);

  const run = async () => {
    setRunning(true);
    setReport(null);
    try {
      const r = await api.doctor({
        outputDir: dir,
        fix: fix && !locked,
        cleanTmp: cleanTmp && !locked,
      });
      setReport(r);
      if (r.repairBlocked) toast(t("Nothing was changed: downloads in progress are using this folder."), "info");
      else if (r.fixed) toast(t("State repaired"), "success");
      else if (!r.hasIssues) toast(t("All files consistent"), "success");
      else toast(t("{n} issue(s) found", { n: r.issues?.length || 0 }), "info");
    } catch (e: any) {
      toast(e.message || t("Doctor failed"), "error");
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <header>
        {/* The page is reached from Settings now, not the rail — so it carries
            its own way back, the way any nested view should. */}
        <button
          type="button"
          onClick={() => pushRoute({ page: "settings" })}
          className="mb-2 flex items-center gap-1.5 text-xs font-medium text-slate-500 transition hover:text-slate-300"
        >
          <ArrowLeft className="h-3.5 w-3.5" /> {t("Settings")}
        </button>
        <h1 className="text-2xl font-bold text-slate-100">{t("Doctor")}</h1>
        <p className="mt-1 text-sm text-slate-400">
          {t("Verify downloaded files against the state file and repair inconsistencies.")}
        </p>
      </header>

      <div className="card space-y-4 p-5">
        <Field label={t("Folder to check")}>
          <button className="input flex items-center gap-2 text-left" onClick={() => setPickDir(true)} type="button">
            <FolderOpen className="h-4 w-4 shrink-0 text-gold-400" />
            <span className="truncate font-mono text-xs">{dir || t("Choose…")}</span>
          </button>
        </Field>

        <div className="grid gap-2 sm:grid-cols-2">
          <Toggle
            label={t("Repair")}
            hint={t("Remove broken entries & files")}
            checked={fix && !locked}
            onChange={setFix}
            disabled={locked}
          />
          <Toggle
            label={t("Clean .tmp")}
            hint={t("Delete orphan temp files")}
            checked={cleanTmp && !locked}
            onChange={setCleanTmp}
            disabled={locked}
          />
        </div>

        {locked && (
          <div className="flex items-start gap-2 rounded-xl border border-amber-400/25 bg-amber-400/[0.07] px-3.5 py-2.5 text-sm text-amber-200">
            <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" />
            <span>
              {t("Unfinished downloads are using this folder ({n}) — repair and cleanup are off.", {
                n: busyJobs.length,
              })}{" "}
              <span className="text-amber-200/70">
                {t(
                  "Their temp files hold what's already downloaded — cleanup would wipe that progress. Finish or cancel them, or just check the folder without repairing.",
                )}
              </span>
            </span>
          </div>
        )}

        <button className="btn-primary" onClick={run} disabled={running || !dir}>
          {running ? <Spinner className="h-4 w-4" /> : <Stethoscope className="h-4 w-4" />}
          {t("Run doctor")}
        </button>
      </div>

      {report && (
        <div className="card animate-fade-in space-y-4 p-5">
          <div className="grid grid-cols-3 gap-3">
            <Stat label={t("In state")} value={report.totalInState} />
            <Stat label={t("Healthy")} value={report.healthy} tone="green" />
            <Stat label={t("Issues")} value={report.issues?.length || 0} tone={report.hasIssues ? "rose" : "slate"} />
          </div>

          {report.seriesTitle && (
            <p className="text-sm text-slate-400">
              {t("Series:")} <span className="text-slate-200">{report.seriesTitle}</span>
            </p>
          )}
          <p className="truncate font-mono text-xs text-slate-600">{report.stateFile}</p>

          {!report.hasIssues ? (
            <div className="flex items-center gap-2 rounded-xl border border-emerald-500/20 bg-emerald-500/[0.07] px-4 py-3 text-sm text-emerald-300">
              <CheckCircle2 className="h-4 w-4" /> {t("All files are consistent with the state file.")}
            </div>
          ) : (
            <>
              {/* The server refuses repairs on a folder a live download owns. It
                  can decide that after the click too — a job may have started
                  while the check was running — so the report says so as well. */}
              {report.repairBlocked && (
                <div className="flex items-start gap-2 rounded-xl border border-amber-400/25 bg-amber-400/[0.07] px-4 py-3 text-sm text-amber-200">
                  <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" />
                  {t("Nothing was changed: downloads in progress are using this folder.")}
                </div>
              )}
              {report.fixed && (
                <div className="flex items-center gap-2 rounded-xl border border-gold-500/20 bg-gold-500/[0.07] px-4 py-3 text-sm text-gold-200">
                  <Wrench className="h-4 w-4" /> {t("State repaired — run the download again to re-fetch affected episodes.")}
                </div>
              )}
              <div className="space-y-1.5">
                {report.issues?.map((iss, i) => (
                  <div key={i} className="rounded-xl border border-white/[0.06] bg-ink-900/40 px-3.5 py-2.5">
                    <div className="flex items-center justify-between gap-2">
                      <span className="chip border-ember-500/25 bg-ember-500/10 text-ember-400">
                        {t(KIND_LABEL[iss.kind] || iss.kind)}
                      </span>
                      {iss.key && <span className="font-mono text-xs text-slate-500">{iss.key}</span>}
                    </div>
                    <p className="mt-1.5 break-words text-sm text-slate-400">{iss.detail}</p>
                  </div>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      <DirPicker open={pickDir} initial={dir} onClose={() => setPickDir(false)} onSelect={setDir} />
    </div>
  );
}

function Stat({ label, value, tone = "default" }: { label: string; value: number; tone?: "default" | "green" | "rose" | "slate" }) {
  const cls = {
    default: "text-slate-100",
    green: "text-emerald-300",
    rose: "text-ember-400",
    slate: "text-slate-400",
  }[tone];
  return (
    <div className="rounded-xl border border-white/[0.06] bg-ink-900/40 px-4 py-3">
      <div className={`text-2xl font-bold tabular-nums ${cls}`}>{value}</div>
      <div className="mt-0.5 text-xs uppercase tracking-wide text-slate-500">{label}</div>
    </div>
  );
}
