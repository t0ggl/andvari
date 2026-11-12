import React, { useEffect, useState } from 'react';
import { Folder, File, Download, Trash2, ChevronRight, Home, RefreshCw } from 'lucide-react';
import { ExplorerResult, FileItem } from '../types';

interface Props {
  effectiveUsername: string;
}

function formatBytes(bytes: number, decimals = 1) {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

function formatDate(isoDate: string) {
  try {
    const d = new Date(isoDate);
    return d.toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    return isoDate;
  }
}

export const FileExplorer: React.FC<Props> = ({ effectiveUsername }) => {
  const [currentPath, setCurrentPath] = useState('');
  const [data, setData] = useState<ExplorerResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadFiles = async (path: string) => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/api/files?path=${encodeURIComponent(path)}`);
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || 'Failed to load directory');
      }
      const json: ExplorerResult = await res.json();
      setData(json);
      setCurrentPath(path);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadFiles(currentPath);
  }, [effectiveUsername]);

  const handleNavigate = (path: string) => {
    loadFiles(path);
  };

  const handleDownload = (item: FileItem) => {
    const url = `/api/files/download?path=${encodeURIComponent(item.path)}`;
    window.open(url, '_blank');
  };

  const handleDelete = async (item: FileItem) => {
    const msg = item.is_dir
      ? `Are you sure you want to permanently delete folder "${item.name}" and all its contents?`
      : `Are you sure you want to delete file "${item.name}"?`;

    if (!confirm(msg)) return;

    try {
      const res = await fetch(`/api/files?path=${encodeURIComponent(item.path)}`, {
        method: 'DELETE',
      });
      if (!res.ok) {
        throw new Error('Failed to delete item');
      }
      loadFiles(currentPath);
    } catch (err: any) {
      alert(err.message);
    }
  };

  return (
    <div className="space-y-4">
      {/* Top action bar: breadcrumbs & refresh */}
      <div className="flex items-center justify-between gap-4 p-3 bg-slate-900 border border-slate-800 rounded-xl">
        {/* Breadcrumbs */}
        <div className="flex items-center gap-1.5 text-xs text-slate-300 overflow-x-auto py-1">
          <button
            onClick={() => handleNavigate('')}
            className={`flex items-center gap-1 px-2 py-1 rounded hover:bg-slate-800 transition ${
              currentPath === '' ? 'text-sky-400 font-semibold' : 'text-slate-400'
            }`}
          >
            <Home className="w-3.5 h-3.5" />
            <span>Root</span>
          </button>

          {data?.breadcrumbs && data.breadcrumbs.map((crumb, idx) => {
            const crumbPath = data.breadcrumbs.slice(0, idx + 1).join('/');
            const isLast = idx === data.breadcrumbs.length - 1;
            return (
              <React.Fragment key={crumbPath}>
                <ChevronRight className="w-3.5 h-3.5 text-slate-600 flex-shrink-0" />
                <button
                  onClick={() => handleNavigate(crumbPath)}
                  className={`px-2 py-1 rounded hover:bg-slate-800 transition ${
                    isLast ? 'text-sky-400 font-semibold' : 'text-slate-400'
                  }`}
                >
                  {crumb}
                </button>
              </React.Fragment>
            );
          })}
        </div>

        {/* Refresh button */}
        <button
          onClick={() => loadFiles(currentPath)}
          disabled={loading}
          className="p-1.5 text-slate-400 hover:text-white hover:bg-slate-800 rounded-lg transition"
          title="Refresh directory"
        >
          <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin text-sky-400' : ''}`} />
        </button>
      </div>

      {/* Error display */}
      {error && (
        <div className="p-4 bg-rose-500/10 border border-rose-500/30 rounded-xl text-xs text-rose-400">
          {error}
        </div>
      )}

      {/* File Table */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        <table className="w-full text-left text-xs">
          <thead className="bg-slate-900/80 border-b border-slate-800 text-slate-400 uppercase tracking-wider font-semibold">
            <tr>
              <th className="py-3 px-4">Name</th>
              <th className="py-3 px-4 w-28 text-right hidden sm:table-cell">Size</th>
              <th className="py-3 px-4 w-44 text-right hidden md:table-cell">Modified</th>
              <th className="py-3 px-4 w-24 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800/60">
            {data && data.items && data.items.length > 0 ? (
              data.items.map((item) => (
                <tr
                  key={item.path}
                  className="hover:bg-slate-800/40 transition group"
                >
                  {/* Name + Icon */}
                  <td className="py-3 px-4">
                    {item.is_dir ? (
                      <button
                        onClick={() => handleNavigate(item.path)}
                        className="flex items-center gap-2.5 text-sky-300 hover:text-sky-200 font-medium text-left"
                      >
                        <Folder className="w-4 h-4 text-sky-400 flex-shrink-0 fill-sky-400/20" />
                        <span className="truncate">{item.name}</span>
                      </button>
                    ) : (
                      <div className="flex items-center gap-2.5 text-slate-200">
                        <File className="w-4 h-4 text-slate-400 flex-shrink-0" />
                        <span className="truncate">{item.name}</span>
                      </div>
                    )}
                  </td>

                  {/* Size */}
                  <td className="py-3 px-4 text-right font-mono text-slate-400 hidden sm:table-cell">
                    {item.is_dir ? '-' : formatBytes(item.size)}
                  </td>

                  {/* Modified */}
                  <td className="py-3 px-4 text-right text-slate-400 hidden md:table-cell">
                    {formatDate(item.mod_time)}
                  </td>

                  {/* Actions: Download & Delete only */}
                  <td className="py-3 px-4 text-right">
                    <div className="flex items-center justify-end gap-1">
                      {!item.is_dir && (
                        <button
                          onClick={() => handleDownload(item)}
                          className="p-1.5 text-slate-400 hover:text-sky-300 hover:bg-sky-500/10 rounded transition"
                          title="Download file"
                        >
                          <Download className="w-3.5 h-3.5" />
                        </button>
                      )}
                      <button
                        onClick={() => handleDelete(item)}
                        className="p-1.5 text-slate-400 hover:text-rose-400 hover:bg-rose-500/10 rounded transition"
                        title={item.is_dir ? 'Delete folder' : 'Delete file'}
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))
            ) : (
              <tr>
                <td colSpan={4} className="py-12 text-center text-slate-500 text-xs">
                  {loading ? 'Loading files...' : 'Folder is empty.'}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
};
