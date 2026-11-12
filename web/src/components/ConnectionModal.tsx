import React, { useState } from 'react';
import { Copy, Check, ShieldCheck, X } from 'lucide-react';
import { UserInfoResponse } from '../types';

interface Props {
  user: UserInfoResponse;
  rawToken?: string;
  isOpen: boolean;
  onClose: () => void;
  onDismissFirstLogin?: () => void;
}

export const ConnectionModal: React.FC<Props> = ({
  user,
  rawToken,
  isOpen,
  onClose,
  onDismissFirstLogin,
}) => {
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'quick' | 'macos' | 'windows' | 'linux' | 'rclone'>('quick');

  if (!isOpen) return null;

  const webdavUrl = `${window.location.origin}/webdav/`;
  const username = user.effective_username;

  const copyToClipboard = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  const handleClose = () => {
    if (onDismissFirstLogin) {
      onDismissFirstLogin();
    }
    onClose();
  };

  const rcloneConfig = `[andvari]
type = webdav
url = ${webdavUrl}
vendor = other
user = ${username}
pass = <run: rclone obscure YOUR_TOKEN>`;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="relative w-full max-w-2xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800 bg-slate-900/60">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-sky-500/10 text-sky-400 rounded-lg">
              <ShieldCheck className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-lg font-semibold text-white">How to Connect via WebDAV</h2>
              <p className="text-xs text-slate-400">Use these credentials to mount your remote storage</p>
            </div>
          </div>
          <button
            onClick={handleClose}
            className="p-1 text-slate-400 hover:text-slate-200 rounded-lg hover:bg-slate-800 transition"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 overflow-y-auto space-y-6">
          {rawToken && (
            <div className="p-4 bg-amber-500/10 border border-amber-500/30 rounded-lg space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold uppercase tracking-wider text-amber-400">
                  New Token Generated (Save Now)
                </span>
                <span className="text-xs text-amber-300/80">Only shown once</span>
              </div>
              <div className="flex items-center justify-between gap-2 p-2 bg-slate-950/80 border border-amber-500/20 rounded font-mono text-sm text-amber-200 break-all select-all">
                <span>{rawToken}</span>
                <button
                  onClick={() => copyToClipboard(rawToken, 'raw-token')}
                  className="p-1.5 hover:bg-amber-500/20 rounded text-amber-300 transition flex-shrink-0"
                  title="Copy Token"
                >
                  {copiedKey === 'raw-token' ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
                </button>
              </div>
            </div>
          )}

          {/* Quick Credential Cards */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-sm">
            <div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1">
              <span className="text-xs text-slate-400">WebDAV URL</span>
              <div className="flex items-center justify-between font-mono text-xs text-slate-200">
                <span className="truncate pr-2">{webdavUrl}</span>
                <button
                  onClick={() => copyToClipboard(webdavUrl, 'url')}
                  className="p-1 text-slate-400 hover:text-white transition"
                  title="Copy URL"
                >
                  {copiedKey === 'url' ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              </div>
            </div>

            <div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1">
              <span className="text-xs text-slate-400">Username</span>
              <div className="flex items-center justify-between font-mono text-xs text-slate-200">
                <span>{username}</span>
                <button
                  onClick={() => copyToClipboard(username, 'user')}
                  className="p-1 text-slate-400 hover:text-white transition"
                  title="Copy Username"
                >
                  {copiedKey === 'user' ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              </div>
            </div>
          </div>

          {/* Guide Tabs */}
          <div>
            <div className="flex items-center gap-2 border-b border-slate-800 text-xs font-medium text-slate-400 mb-4">
              {[
                { id: 'quick', label: 'Quick Start' },
                { id: 'macos', label: 'macOS Finder' },
                { id: 'windows', label: 'Windows Explorer' },
                { id: 'linux', label: 'Linux (GNOME)' },
                { id: 'rclone', label: 'rclone' },
              ].map((tab) => (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id as any)}
                  className={`pb-2 px-1 border-b-2 transition ${
                    activeTab === tab.id
                      ? 'border-sky-500 text-sky-400'
                      : 'border-transparent hover:text-slate-200'
                  }`}
                >
                  {tab.label}
                </button>
              ))}
            </div>

            <div className="text-xs text-slate-300 space-y-2">
              {activeTab === 'quick' && (
                <ul className="list-disc list-inside space-y-1.5 text-slate-300">
                  <li>Any RFC 4918 WebDAV client is supported (Finder, Cyberduck, Nextcloud, rclone, WinSCP).</li>
                  <li>Always use <strong>HTTP Basic Authentication</strong>.</li>
                  <li>Your username is <code className="px-1 py-0.5 bg-slate-800 text-sky-300 rounded">{username}</code>.</li>
                  <li>Your password is the active token generated in your dashboard.</li>
                  <li>You can have up to 2 active tokens at any time to permit zero-downtime rotation.</li>
                </ul>
              )}

              {activeTab === 'macos' && (
                <ol className="list-decimal list-inside space-y-2">
                  <li>In <strong>Finder</strong>, press <kbd className="px-1 py-0.5 bg-slate-800 rounded">⌘K</kbd> or click <strong>Go &gt; Connect to Server...</strong></li>
                  <li>Paste the WebDAV URL: <code className="px-1 py-0.5 bg-slate-800 text-sky-300 rounded">{webdavUrl}</code></li>
                  <li>Click <strong>Connect</strong>.</li>
                  <li>Select <strong>Registered User</strong> and enter your username and token password.</li>
                </ol>
              )}

              {activeTab === 'windows' && (
                <ol className="list-decimal list-inside space-y-2">
                  <li>Open <strong>File Explorer</strong>, right-click <strong>This PC</strong>, and select <strong>Map network drive...</strong></li>
                  <li>Click <strong>Connect to a Web site that you can use to store your documents &amp; pictures</strong>.</li>
                  <li>Enter the URL: <code className="px-1 py-0.5 bg-slate-800 text-sky-300 rounded">{webdavUrl}</code></li>
                  <li>When prompted, enter your username and token password.</li>
                </ol>
              )}

              {activeTab === 'linux' && (
                <ol className="list-decimal list-inside space-y-2">
                  <li>Open <strong>Files</strong> (Nautilus) and click <strong>+ Other Locations</strong> in the sidebar.</li>
                  <li>In <strong>Connect to Server</strong>, enter: <code className="px-1 py-0.5 bg-slate-800 text-sky-300 rounded">{webdavUrl.replace(/^http/, 'dav')}</code></li>
                  <li>Click <strong>Connect</strong> and enter your username and token password.</li>
                </ol>
              )}

              {activeTab === 'rclone' && (
                <div className="space-y-2">
                  <p>Add this remote to your <code className="text-sky-300">~/.config/rclone/rclone.conf</code>:</p>
                  <pre className="p-3 bg-slate-950 border border-slate-800 rounded text-slate-300 overflow-x-auto font-mono text-xs">
                    {rcloneConfig}
                  </pre>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end px-6 py-4 border-t border-slate-800 bg-slate-900/60">
          <button
            onClick={handleClose}
            className="px-4 py-2 bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold rounded-lg shadow transition"
          >
            I've Saved My Credentials
          </button>
        </div>
      </div>
    </div>
  );
};
