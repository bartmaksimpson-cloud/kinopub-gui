import clsx from "clsx";
import { CalendarClock, Loader2, User as UserIcon, WifiOff } from "lucide-react";
import { useApp } from "../store";
import { useI18n } from "../i18n";
import { KinopubLogin } from "../components/KinopubLogin";

// ProfilePage is where the sidebar account card leads: the kino.watch account and
// its subscription, plus sign-in/sign-out. App preferences live on their own
// Settings page (a separate nav entry), so neither destination hides the other.
export function ProfilePage() {
  const { kpauth, kpUser, kpUserError } = useApp();
  const { t } = useI18n();

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <header>
        <h1 className="text-2xl font-bold text-slate-100">{t("Profile")}</h1>
        <p className="mt-1 text-sm text-slate-400">{t("Your kino.watch account and subscription.")}</p>
      </header>

      {kpauth.loggedIn && <AccountCard user={kpUser} error={kpUserError} />}

      <KinopubLogin />
    </div>
  );
}

// AccountCard mirrors the sidebar card in full size. The three states are the
// same ones the sidebar distinguishes: profile loaded, still loading, and
// kino.watch unreachable (e.g. VPN off) — which must not read as "no subscription".
function AccountCard({
  user,
  error,
}: {
  user: ReturnType<typeof useApp>["kpUser"];
  error: boolean;
}) {
  const { t } = useI18n();

  if (!user) {
    return (
      <div className="card flex items-center gap-3.5 p-5">
        <div
          className={clsx(
            "grid h-12 w-12 shrink-0 place-items-center rounded-full bg-ink-800 ring-2",
            error ? "ring-amber-400/60" : "ring-slate-600/60",
          )}
        >
          {error ? (
            <WifiOff className="h-5 w-5 text-amber-300" />
          ) : (
            <Loader2 className="h-5 w-5 animate-spin text-slate-400" />
          )}
        </div>
        <div className="min-w-0">
          <div className="text-base font-semibold text-slate-200">{t("Signed in")}</div>
          <div className={clsx("text-sm font-medium", error ? "text-amber-300" : "text-slate-500")}>
            {error ? t("Can't reach kino.watch") : t("Checking subscription…")}
          </div>
        </div>
      </div>
    );
  }

  const active = user.subscriptionActive;
  const days = user.subscriptionDays;
  const ring = !active ? "ring-ember-500/60" : days <= 14 ? "ring-amber-400/70" : "ring-emerald-400/70";
  const subText = !active ? "text-ember-400" : days <= 14 ? "text-amber-300" : "text-emerald-400";
  const end = user.subscriptionEnd ? new Date(user.subscriptionEnd * 1000) : null;

  return (
    <div className="card space-y-4 p-5">
      <div className="flex items-center gap-3.5">
        <div className={clsx("grid h-12 w-12 shrink-0 place-items-center overflow-hidden rounded-full bg-ink-800 ring-2", ring)}>
          {user.avatar ? (
            <img src={user.avatar} alt="" className="h-full w-full object-cover" />
          ) : (
            <UserIcon className="h-5 w-5 text-slate-300" />
          )}
        </div>
        <div className="min-w-0">
          <div className="truncate text-base font-semibold text-slate-200">{user.username || t("Signed in")}</div>
          <div className={clsx("text-sm font-medium", subText)}>
            {active ? t("{n} days left", { n: days }) : t("No subscription")}
          </div>
        </div>
      </div>

      {end && !Number.isNaN(end.getTime()) && (
        <div className="flex items-center gap-2.5 border-t border-white/[0.06] pt-3 text-sm">
          <CalendarClock className="h-4 w-4 shrink-0 text-gold-400" />
          <span className="text-slate-400">{t("Subscription ends")}</span>
          <span className="ml-auto font-medium text-slate-300">{end.toLocaleDateString()}</span>
        </div>
      )}
    </div>
  );
}
