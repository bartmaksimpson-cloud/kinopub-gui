import { useEffect, useState } from "react";
import { ArrowUp, Check, CornerDownLeft, Folder, FolderOpen, HardDrive } from "lucide-react";
import { api, isNavigationAbort, type FSListing } from "../api";
import { useI18n } from "../i18n";
import { Modal, Spinner } from "./ui";

export function DirPicker({
  open,
  initial,
  onClose,
  onSelect,
}: {
  open: boolean;
  initial?: string;
  onClose: () => void;
  onSelect: (path: string) => void;
}) {
  const { t } = useI18n();
  const [listing, setListing] = useState<FSListing | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [typed, setTyped] = useState("");
  const [checking, setChecking] = useState(false);

  const load = (path: string) => {
    setLoading(true);
    setError("");
    api
      .fs(path)
      .then((l) => {
        setListing(l);
        setTyped(l.path);
      })
      .catch((e) => !isNavigationAbort(e) && setError(String(e.message || e)))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    if (open) load(initial || "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  return (
    <Modal open={open} onClose={onClose} title={t("Choose a folder")}>
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <button
            className="btn-ghost px-3 py-2"
            onClick={() => listing && load(listing.parent)}
            disabled={!listing || loading}
            title={t("Parent folder")}
          >
            <ArrowUp className="h-4 w-4" />
          </button>
          {/* Путь можно вписать руками: так открывается сетевая папка, которую
              не пройти кликами — например \\192.168.1.174\Video на Windows или
              /Volumes/NAS на macOS. */}
          <input
            className="input flex-1 font-mono text-xs"
            value={typed}
            placeholder={listing?.path || "…"}
            onChange={(e) => setTyped(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && typed.trim()) load(typed.trim());
            }}
            spellCheck={false}
          />
          <button
            className="btn-ghost px-3 py-2"
            disabled={!typed.trim() || loading}
            onClick={() => load(typed.trim())}
            title={t("Open this path")}
          >
            <CornerDownLeft className="h-4 w-4" />
          </button>
        </div>

        {listing?.places && listing.places.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {listing.places.map((p) => (
              <button
                key={p.path}
                onClick={() => load(p.path)}
                className="inline-flex items-center gap-1.5 rounded-full border border-white/[0.08] bg-white/[0.02] px-3 py-1.5 text-xs text-slate-400 transition hover:border-white/20 hover:text-slate-200"
              >
                <HardDrive className="h-3.5 w-3.5 shrink-0 text-gold-400/80" />
                {p.name}
              </button>
            ))}
          </div>
        )}

        <div className="h-64 overflow-y-auto rounded-xl border border-white/[0.06] bg-ink-900/40 p-1.5">
          {loading ? (
            <div className="flex h-full items-center justify-center text-slate-500">
              <Spinner className="h-5 w-5" />
            </div>
          ) : error ? (
            <div className="p-4 text-sm text-ember-400">{error}</div>
          ) : listing && listing.dirs.length > 0 ? (
            listing.dirs.map((d) => (
              <button
                key={d.path}
                onClick={() => load(d.path)}
                className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm text-slate-300 hover:bg-white/[0.05]"
              >
                <Folder className="h-4 w-4 shrink-0 text-gold-400/80" />
                <span className="truncate">{d.name}</span>
              </button>
            ))
          ) : (
            <div className="p-4 text-sm text-slate-500">{t("No sub-folders here.")}</div>
          )}
        </div>

        <div className="flex items-center justify-between gap-3 pt-1">
          <span className="text-xs text-slate-500">
            <FolderOpen className="mr-1 inline h-3.5 w-3.5" />
            {t("Files download into this folder.")}
          </span>
          <button
            className="btn-primary"
            disabled={!listing || checking}
            onClick={async () => {
              if (!listing) return;
              // Сетевая папка часто прекрасно открывается и не принимает запись.
              // Лучше сказать об этом сейчас, чем после скачанного фильма.
              setChecking(true);
              try {
                const r = await api.checkDir(listing.path);
                if (!r.ok) {
                  setError(r.error || t("This folder cannot be written to."));
                  return;
                }
                onSelect(listing.path);
                onClose();
              } catch (e: any) {
                setError(String(e.message || e));
              } finally {
                setChecking(false);
              }
            }}
          >
            {checking ? <Spinner className="h-4 w-4" /> : <Check className="h-4 w-4" />}
            {t("Use this folder")}
          </button>
        </div>
      </div>
    </Modal>
  );
}
