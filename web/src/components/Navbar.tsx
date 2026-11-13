import React, { useEffect, useState } from 'react';
import { UserInfoResponse } from '../types';
import { HelpCircle, LogOut, Shield, Users } from 'lucide-react';

interface Props {
  user: UserInfoResponse;
  onOpenHelp: () => void;
  onImpersonate: (targetUser: string) => void;
  onUnimpersonate: () => void;
  onLogout: () => void;
}

export const Navbar: React.FC<Props> = ({
  user,
  onOpenHelp,
  onImpersonate,
  onUnimpersonate,
  onLogout,
}) => {
  const [allUsers, setAllUsers] = useState<string[]>([]);

  useEffect(() => {
    if (user.is_admin) {
      fetch('/api/admin/users')
        .then((res) => res.json())
        .then((data) => {
          if (data.users) setAllUsers(data.users);
        })
        .catch(() => {});
    }
  }, [user.is_admin]);

  return (
    <header className="sticky top-0 z-40 w-full border-b border-slate-800 bg-slate-950/80 backdrop-blur">
      <div className="max-w-7xl mx-auto px-3 sm:px-6 lg:px-8 h-16 flex items-center justify-between gap-2 sm:gap-4">
        {/* Brand */}
        <div className="flex items-center gap-2.5 sm:gap-3 shrink-0">
          <img src="/icon.svg" alt="Andvari" className="w-8 h-8 rounded-lg shadow-sm shrink-0" />
          <div className="flex flex-col">
            <span className="text-base font-bold text-white tracking-tight leading-tight">Andvari</span>
            <span className="text-[10px] text-slate-400 hidden sm:block">Personal Cloud Storage</span>
          </div>
        </div>

        {/* Right actions */}
        <div className="flex items-center gap-1.5 sm:gap-3 min-w-0 shrink">
          {/* Admin Impersonation Selector */}
          {user.is_admin && (
            <div
              className={`flex items-center gap-1.5 sm:gap-2 border rounded-lg px-2 sm:px-2.5 py-1 text-xs min-w-0 shrink max-w-[125px] sm:max-w-xs transition ${
                user.is_impersonating
                  ? 'bg-amber-950/40 border-amber-500/40 text-amber-200'
                  : 'bg-slate-900 border-slate-800 text-slate-200'
              }`}
            >
              <Users className={`w-3.5 h-3.5 shrink-0 ${user.is_impersonating ? 'text-amber-400' : 'text-sky-400'}`} />
              <span className="text-slate-400 hidden sm:inline shrink-0">Impersonate:</span>
              <select
                value={user.is_impersonating ? user.effective_username : ''}
                aria-label="Select tenant to impersonate"
                onChange={(e) => {
                  const val = e.target.value;
                  if (!val) {
                    onUnimpersonate();
                  } else {
                    onImpersonate(val);
                  }
                }}
                className="bg-transparent text-xs text-inherit focus:outline-none cursor-pointer truncate min-w-0 w-full"
              >
                <option value="" className="bg-slate-900 text-slate-300">
                  Self ({user.username})
                </option>
                {allUsers
                  .filter((u) => u !== user.username)
                  .map((u) => (
                    <option key={u} value={u} className="bg-slate-900 text-slate-300">
                      User: {u}
                    </option>
                  ))}
              </select>
            </div>
          )}

          {/* Help Button */}
          <button
            onClick={onOpenHelp}
            aria-label="How to Connect via WebDAV"
            className="flex items-center gap-1.5 p-1.5 sm:px-3 sm:py-1.5 bg-slate-900 hover:bg-slate-800 border border-slate-800 rounded-lg text-xs font-medium text-slate-300 transition shrink-0"
          >
            <HelpCircle className="w-3.5 h-3.5 text-sky-400 shrink-0" />
            <span className="hidden sm:inline">How to Connect</span>
          </button>

          {/* User badge */}
          <div className="flex items-center gap-2 pl-1.5 sm:pl-2 border-l border-slate-800 text-xs text-slate-300 shrink-0">
            <div className="w-7 h-7 rounded-full bg-slate-800 flex items-center justify-center font-bold text-sky-400 shrink-0">
              {user.username.charAt(0).toUpperCase()}
            </div>
            <div className="hidden md:flex flex-col text-left">
              <div className="flex items-center gap-1">
                <span className="font-semibold text-slate-200">{user.username}</span>
                {user.is_admin && (
                  <span title="Admin User">
                    <Shield className="w-3 h-3 text-amber-400" />
                  </span>
                )}
              </div>
            </div>
          </div>

          {/* Logout */}
          <button
            onClick={onLogout}
            aria-label="Log Out"
            className="p-1.5 sm:p-2 text-slate-400 hover:text-rose-400 hover:bg-slate-900 rounded-lg transition shrink-0"
            title="Log Out"
          >
            <LogOut className="w-4 h-4 shrink-0" />
          </button>
        </div>
      </div>
    </header>
  );
};
