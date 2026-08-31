import { useState } from "react";
import clsx from "clsx";
import { Download, Loader2 } from "lucide-react";
import { api } from "../api";
import { useApp } from "../store";
import { useI18n } from "../i18n";

// InstallFFmpeg shows a one-click "Install ffmpeg" button when ffmpeg is missing
// and an automatic install is available for this platform. The server downloads
// a static build into its config dir and broadcasts the new status over SSE, so
// the indicator turns green without a reload.
export function InstallFFmpeg({ className }: { className?: string }) {
  const { ffmpeg, ffmpegInstall, toast } = useApp();
  const { t } = useI18n();
  const [installing, setInstalling] = useState(false);

  if (ffmpeg.ffmpegFound || !ffmpegInstall.supported) return null;

  const install = async () => {
    setInstalling(true);
    try {
      await api.installDeps();
      toast(t("ffmpeg installed."), "success");
    } catch (e: any) {
      toast(e.message || t("ffmpeg install failed"), "error");
    } finally {
      setInstalling(false);
    }
  };

  return (
    <div className={clsx("space-y-1.5", className)}>
      <button className="btn-primary" onClick={install} disabled={installing}>
        {installing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}
        {installing ? t("Installing ffmpeg…") : t("Install ffmpeg")}
      </button>
      {/* Only the wait hint while a download is in flight. Naming the upstream
          build host up front told the user nothing they could act on. */}
      {installing && <p className="text-xs text-slate-500">{t("Downloading a static build — this can take a minute.")}</p>}
    </div>
  );
}
