import React, { useEffect, useState } from 'react';
import { Folder, File, Download, Trash2, ChevronRight, Home, RefreshCw, AlertTriangle } from 'lucide-react';
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

  // Styled modal state replacing native confirm/alert
  const [itemToDelete, setItemToDelete] = useState<FileItem | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

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

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && itemToDelete) {
        setItemToDelete(null);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [itemToDelete]);

  const handleNavigate = (path: string) => {
    loadFiles(path);
  };

  const handleDownload = (item: FileItem) => {
    const url = `/api/files/download?path=${encodeURIComponent(item.path)}`;
    window.open(url, '_blank');
  };

  const handleConfirmDelete = async () => {
    if (!itemToDelete) return;
    setDeleteLoading(true);
    setError(null);

    try {
      const res = await fetch(`/api/files?path=${encodeURIComponent(itemToDelete.path)}`, {
        method: 'DELETE',
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || 'Failed to delete item');
      }
      setItemToDelete(null);
      loadFiles(currentPath);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setDeleteLoading(false);
    }
  };

  return (
    <div className="space-y-4">
      {/* Top action bar: breadcrumbs & refresh */}
      <div className="flex items-center justify-between gap-4 p-3 bg-slate-900 border border-slate-800 rounded-xl">
        {/* Breadcrumbs */}
        <nav aria-label="Directory navigation" className="flex items-center gap-1.5 text-xs text-slate-300 overflow-x-auto py-1">
          <button
            onClick={() => handleNavigate('')}
            aria-label="Navigate to root directory"
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
                  aria-label={`Navigate to ${crumb}`}
                  className={`px-2 py-1 rounded hover:bg-slate-800 transition ${
                    isLast ? 'text-sky-400 font-semibold' : 'text-slate-400'
                  }`}
                >
                  {crumb}
                </button>
              </React.Fragment>
            );
          })}
        </nav>

        {/* Refresh button */}
        <button
          onClick={() => loadFiles(currentPath)}
          disabled={loading}
          aria-label="Refresh directory contents"
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

                  {/* Actions: Download & Delete */}
                  <td className="py-3 px-4 text-right">
                    <div className="flex items-center justify-end gap-1">
                      {!item.is_dir && (
                        <button
                          onClick={() => handleDownload(item)}
                          aria-label={`Download file ${item.name}`}
                          className="p-1.5 text-slate-400 hover:text-sky-300 hover:bg-sky-500/10 rounded transition"
                          title="Download file"
                        >
                          <Download className="w-3.5 h-3.5" />
                        </button>
                      )}
                      <button
                        onClick={() => setItemToDelete(item)}
                        aria-label={`Delete ${item.is_dir ? 'folder' : 'file'} ${item.name}`}
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
                <td colSpan={4} className="py-14 text-center">
                  <div className="flex flex-col items-center justify-center gap-2 max-w-sm mx-auto text-slate-500">
                    <div className="w-10 h-10 rounded-full bg-slate-800/80 flex items-center justify-center text-slate-400 mb-1">
                      <Folder className="w-5 h-5" />
                    </div>
                    <span className="text-sm font-medium text-slate-300">
                      {loading ? 'Loading files...' : 'Folder is empty'}
                    </span>
                    {!loading && (
                      <p className="text-xs text-slate-400">
                        Mount your WebDAV storage in Finder, Explorer, or rclone to upload and sync files.
                      </p>
                    )}
                  </div>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Delete Confirmation Modal */}
      {itemToDelete && (
        <div
          onClick={(e) => {
            if (e.target === e.currentTarget) setItemToDelete(null);
          }}
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in"
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="delete-dialog-title"
            className="w-full max-w-md bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4"
          >
            <div className="flex items-start gap-3">
              <div className="p-2 bg-rose-500/10 text-rose-400 rounded-lg">
                <AlertTriangle className="w-5 h-5 flex-shrink-0" />
              </div>
              <div>
                <h3 id="delete-dialog-title" className="text-base font-semibold text-white">
                  Delete {itemToDelete.is_dir ? 'Folder' : 'File'}
                </h3>
                <p className="text-xs text-slate-400 mt-1">
                  Are you sure you want to permanently delete{' '}
                  <strong className="text-slate-200">"{itemToDelete.name}"</strong>
                  {itemToDelete.is_dir ? ' and all of its contents' : ''}? This action cannot be undone.
                </p>
              </div>
            </div>

            <div className="flex items-center justify-end gap-2 pt-2">
              <button
                type="button"
                disabled={deleteLoading}
                onClick={() => setItemToDelete(null)}
                className="px-3 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium rounded-lg transition"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={deleteLoading}
                onClick={handleConfirmDelete}
                className="px-4 py-2 bg-rose-600 hover:bg-rose-500 text-white text-xs font-semibold rounded-lg transition disabled:opacity-50 flex items-center gap-1.5"
              >
                {deleteLoading ? 'Deleting...' : 'Delete'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
