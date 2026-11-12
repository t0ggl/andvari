import React, { useState } from 'react';
import { KeyRound, Plus, Trash2, ShieldCheck, Copy, Check, Clock, Calendar } from 'lucide-react';
import { TokenResponse } from '../types';

interface Props {
  tokens: TokenResponse[];
  effectiveUsername: string;
  onTokenGenerated: () => void;
  onTokenRevoked: () => void;
}

export const TokenManager: React.FC<Props> = ({
  tokens,
  effectiveUsername,
  onTokenGenerated,
  onTokenRevoked,
}) => {
  const [isCreating, setIsCreating] = useState(false);
  const [label, setLabel] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [newRawToken, setNewRawToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const canCreate = tokens.length < 2;

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const res = await fetch('/api/tokens', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ label: label.trim() || `Token ${tokens.length + 1}` }),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || 'Failed to generate token');
      }

      setNewRawToken(data.token);
      setLabel('');
      onTokenGenerated();
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleRevoke = async (id: string, tokenLabel: string) => {
    if (!confirm(`Are you sure you want to revoke "${tokenLabel}"? Any WebDAV clients using this token will be disconnected.`)) {
      return;
    }

    try {
      const res = await fetch(`/api/tokens?id=${encodeURIComponent(id)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error('Failed to revoke token');
      onTokenRevoked();
    } catch (err: any) {
      alert(err.message);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="space-y-6">
      {/* Header and status */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 p-5 bg-slate-900 border border-slate-800 rounded-xl">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <KeyRound className="w-5 h-5 text-sky-400" />
            <h3 className="text-base font-semibold text-white">WebDAV Access Tokens</h3>
            <span
              className={`text-xs px-2 py-0.5 rounded-full font-medium ${
                tokens.length >= 2
                  ? 'bg-amber-500/10 text-amber-400 border border-amber-500/20'
                  : 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              }`}
            >
              {tokens.length} / 2 Active
            </span>
          </div>
          <p className="text-xs text-slate-400">
            Generate and manage access tokens for mounting your WebDAV storage on{' '}
            <span className="text-sky-300 font-mono">{effectiveUsername}</span>. A maximum of 2 tokens are allowed to support zero-downtime rotation.
          </p>
        </div>

        <button
          onClick={() => {
            setIsCreating(true);
            setNewRawToken(null);
            setError(null);
          }}
          disabled={!canCreate}
          className={`flex items-center justify-center gap-2 px-4 py-2 rounded-lg text-xs font-semibold shadow transition flex-shrink-0 ${
            canCreate
              ? 'bg-sky-600 hover:bg-sky-500 text-white cursor-pointer'
              : 'bg-slate-800 text-slate-500 cursor-not-allowed border border-slate-700'
          }`}
          title={!canCreate ? 'Maximum of 2 tokens reached. Revoke one to create a new token.' : ''}
        >
          <Plus className="w-4 h-4" />
          <span>Generate Token</span>
        </button>
      </div>

      {/* Newly Created Token Display Card */}
      {newRawToken && (
        <div className="p-5 bg-emerald-950/40 border border-emerald-500/30 rounded-xl space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-emerald-400">
              <ShieldCheck className="w-4 h-4" />
              <span className="text-xs font-semibold uppercase tracking-wider">
                New Token Created Successfully
              </span>
            </div>
            <button
              onClick={() => setNewRawToken(null)}
              className="text-xs text-slate-400 hover:text-white"
            >
              Dismiss
            </button>
          </div>
          <p className="text-xs text-emerald-200/90">
            Make sure to copy your new token now. For security purposes, it will never be displayed again.
          </p>
          <div className="flex items-center justify-between gap-2 p-3 bg-slate-950 border border-emerald-500/20 rounded-lg font-mono text-sm text-emerald-300 break-all select-all">
            <span>{newRawToken}</span>
            <button
              onClick={() => copyToClipboard(newRawToken)}
              className="flex items-center gap-1.5 px-3 py-1.5 bg-emerald-500/20 hover:bg-emerald-500/30 text-emerald-300 rounded font-sans text-xs font-semibold transition flex-shrink-0"
            >
              {copied ? (
                <>
                  <Check className="w-3.5 h-3.5" />
                  <span>Copied!</span>
                </>
              ) : (
                <>
                  <Copy className="w-3.5 h-3.5" />
                  <span>Copy</span>
                </>
              )}
            </button>
          </div>
        </div>
      )}

      {/* Active Tokens List */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        <div className="px-5 py-3 border-b border-slate-800 bg-slate-900/60 flex items-center justify-between">
          <span className="text-xs font-medium text-slate-400 uppercase tracking-wider">
            Active Tokens
          </span>
          <span className="text-xs text-slate-500">{tokens.length} of 2 slots used</span>
        </div>

        {tokens.length === 0 ? (
          <div className="p-8 text-center text-slate-500 text-xs">
            No active tokens. Click "Generate Token" above to create an access token for WebDAV.
          </div>
        ) : (
          <div className="divide-y divide-slate-800/60">
            {tokens.map((tok) => (
              <div
                key={tok.id}
                className="p-4 sm:p-5 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 hover:bg-slate-800/20 transition"
              >
                <div className="space-y-1.5">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-semibold text-white">{tok.label}</span>
                    <span className="font-mono text-[10px] text-slate-500 bg-slate-950 px-1.5 py-0.5 rounded border border-slate-800">
                      ID: {tok.id.slice(0, 8)}...
                    </span>
                  </div>

                  <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-slate-400">
                    <span className="flex items-center gap-1">
                      <Calendar className="w-3 h-3 text-slate-500" />
                      Created: {tok.created_at}
                    </span>
                    {tok.last_used_at && (
                      <span className="flex items-center gap-1 text-sky-400/80">
                        <Clock className="w-3 h-3 text-sky-400/60" />
                        Last used: {tok.last_used_at}
                      </span>
                    )}
                  </div>
                </div>

                <button
                  onClick={() => handleRevoke(tok.id, tok.label)}
                  className="flex items-center justify-center gap-1.5 px-3 py-1.5 bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 rounded-lg text-xs font-medium transition self-start sm:self-center"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  <span>Revoke</span>
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Creation Modal */}
      {isCreating && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in">
          <div className="w-full max-w-md bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4">
            <h3 className="text-base font-semibold text-white">Generate New Token</h3>
            <p className="text-xs text-slate-400">
              Give your token an identifiable label (e.g. "Work Laptop", "Home Desktop", "Phone").
            </p>

            {error && (
              <div className="p-3 bg-rose-500/10 border border-rose-500/30 rounded text-xs text-rose-400">
                {error}
              </div>
            )}

            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-slate-300 mb-1">
                  Token Label
                </label>
                <input
                  type="text"
                  value={label}
                  onChange={(e) => setLabel(e.target.value)}
                  placeholder={`Token ${tokens.length + 1}`}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-white placeholder-slate-500 focus:outline-none focus:border-sky-500 transition"
                  autoFocus
                />
              </div>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setIsCreating(false)}
                  className="px-3 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium rounded-lg transition"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={loading}
                  className="px-4 py-2 bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold rounded-lg transition disabled:opacity-50"
                >
                  {loading ? 'Generating...' : 'Generate Token'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
