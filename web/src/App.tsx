import React, { useEffect, useState } from 'react';
import { Navbar } from './components/Navbar';
import { ImpersonationBanner } from './components/ImpersonationBanner';
import { TokenManager } from './components/TokenManager';
import { FileExplorer } from './components/FileExplorer';
import { ConnectionModal } from './components/ConnectionModal';
import { UserInfoResponse } from './types';
import { Files, KeyRound, LogIn, ShieldCheck } from 'lucide-react';

export const App: React.FC = () => {
  const [user, setUser] = useState<UserInfoResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<'files' | 'tokens'>('files');
  const [isHelpOpen, setIsHelpOpen] = useState(false);
  const [flashToken, setFlashToken] = useState<string | undefined>(undefined);

  const fetchUser = async () => {
    try {
      const res = await fetch('/api/me');
      if (res.status === 401) {
        setUser(null);
        return;
      }
      const data: UserInfoResponse = await res.json();
      setUser(data);

      if (data.flash_token) {
        setFlashToken(data.flash_token);
        setIsHelpOpen(true);
      } else if (!data.first_login_completed) {
        setIsHelpOpen(true);
      }
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchUser();
  }, []);

  const handleDismissFirstLogin = async () => {
    try {
      await fetch('/api/onboarding/complete', { method: 'POST' });
    } catch {}
  };

  const handleImpersonate = async (targetUser: string) => {
    try {
      const res = await fetch('/api/admin/impersonate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ target_user: targetUser }),
      });
      if (res.ok) fetchUser();
    } catch {}
  };

  const handleUnimpersonate = async () => {
    try {
      const res = await fetch('/api/admin/impersonate', { method: 'DELETE' });
      if (res.ok) fetchUser();
    } catch {}
  };

  const handleLogout = async () => {
    try {
      await fetch('/auth/logout', { method: 'POST' });
    } finally {
      window.location.href = '/auth/login';
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-slate-950 text-slate-400">
        <div className="flex flex-col items-center gap-3">
          <div className="w-8 h-8 border-2 border-sky-500 border-t-transparent rounded-full animate-spin" />
          <span className="text-xs">Loading Andvari...</span>
        </div>
      </div>
    );
  }

  // Unauthenticated view
  if (!user) {
    return (
      <div className="min-h-screen bg-slate-950 flex flex-col justify-center items-center p-4">
        <div className="w-full max-w-sm bg-slate-900 border border-slate-800 rounded-2xl p-8 shadow-2xl space-y-6 text-center">
          <div className="flex justify-center">
            <img src="/icon.svg" alt="Andvari Logo" className="w-16 h-16 rounded-2xl shadow-lg" />
          </div>

          <div className="space-y-1">
            <h1 className="text-xl font-bold text-white tracking-tight">Andvari WebDAV</h1>
            <p className="text-xs text-slate-400">
              Multi-tenant shared cloud storage. Sign in using your single sign-on identity to manage tokens and files.
            </p>
          </div>

          <a
            href="/auth/login"
            className="flex items-center justify-center gap-2 w-full py-2.5 px-4 bg-sky-600 hover:bg-sky-500 text-white rounded-xl text-xs font-semibold shadow-lg shadow-sky-600/20 transition cursor-pointer"
          >
            <LogIn className="w-4 h-4" />
            <span>Sign in with SSO</span>
          </a>

          <div className="flex items-center justify-center gap-1.5 text-[11px] text-slate-500">
            <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />
            <span>Argon2id Hashed · OIDC Secured</span>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col">
      <Navbar
        user={user}
        onOpenHelp={() => setIsHelpOpen(true)}
        onImpersonate={handleImpersonate}
        onUnimpersonate={handleUnimpersonate}
        onLogout={handleLogout}
      />

      {user.is_impersonating && (
        <ImpersonationBanner
          impersonatedUser={user.effective_username}
          onExit={handleUnimpersonate}
        />
      )}

      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-6 space-y-6">
        {/* Navigation tabs */}
        <div className="flex items-center gap-2 border-b border-slate-800 text-sm font-medium">
          <button
            onClick={() => setActiveTab('files')}
            className={`flex items-center gap-2 pb-3 px-2 border-b-2 transition ${
              activeTab === 'files'
                ? 'border-sky-500 text-sky-400 font-semibold'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Files className="w-4 h-4" />
            <span>Files</span>
          </button>

          <button
            onClick={() => setActiveTab('tokens')}
            className={`flex items-center gap-2 pb-3 px-2 border-b-2 transition ${
              activeTab === 'tokens'
                ? 'border-sky-500 text-sky-400 font-semibold'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <KeyRound className="w-4 h-4" />
            <span>WebDAV &amp; Tokens</span>
            <span className="text-[10px] font-mono px-1.5 py-0.2 rounded-full bg-slate-800 text-slate-300">
              {user.tokens.length}/2
            </span>
          </button>
        </div>

        {/* Tab content */}
        {activeTab === 'files' ? (
          <FileExplorer effectiveUsername={user.effective_username} />
        ) : (
          <TokenManager
            tokens={user.tokens}
            effectiveUsername={user.effective_username}
            onTokenGenerated={fetchUser}
            onTokenRevoked={fetchUser}
          />
        )}
      </main>

      <ConnectionModal
        user={user}
        rawToken={flashToken}
        isOpen={isHelpOpen}
        onClose={() => setIsHelpOpen(false)}
        onDismissFirstLogin={handleDismissFirstLogin}
      />
    </div>
  );
};
export default App;
