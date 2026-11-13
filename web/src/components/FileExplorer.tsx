import React, { useEffect, useState, useRef } from 'react';
import { Folder, File, Download, Trash2, ChevronRight, Home, RefreshCw, AlertTriangle, X } from 'lucide-react';
import { ExplorerResult, FileItem } from '../types';

interface Props {
  effectiveUsername: string;
  onFileDeleted?: () => void;
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

export const FileExplorer: React.FC<Props> = ({ effectiveUsername, onFileDeleted }) => {
  const [currentPath, setCurrentPath] = useState('');
  const [data, setData] = useState<ExplorerResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Multi-select state
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());
  const [anchorIndex, setAnchorIndex] = useState<number | null>(null);
  const [selectionBeforeRange, setSelectionBeforeRange] = useState<Set<string>>(new Set());

  // Styled modal state supporting single or batch deletion
  const [itemsToDelete, setItemsToDelete] = useState<FileItem[] | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const headerCheckboxRef = useRef<HTMLInputElement | null>(null);

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

  // Prune any selected paths that are no longer in data.items
  useEffect(() => {
    if (data?.items) {
      const existing = new Set(data.items.map((i) => i.path));
      setSelectedPaths((prev) => {
        let changed = false;
        const next = new Set<string>();
        for (const p of prev) {
          if (existing.has(p)) {
            next.add(p);
          } else {
            changed = true;
          }
        }
        return changed ? next : prev;
      });
    }
  }, [data]);

  // Indeterminate state for master checkbox
  const itemCount = data?.items?.length || 0;
  const selectedCount = selectedPaths.size;
  const allSelected = itemCount > 0 && selectedCount === itemCount;
  const someSelected = selectedCount > 0 && selectedCount < itemCount;

  useEffect(() => {
    if (headerCheckboxRef.current) {
      headerCheckboxRef.current.indeterminate = someSelected;
    }
  }, [someSelected]);

  // Global keydown handler for Escape (close modal / clear selection) and Ctrl+A / Cmd+A
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (itemsToDelete) {
          setItemsToDelete(null);
        } else if (selectedPaths.size > 0) {
          setSelectedPaths(new Set());
          setAnchorIndex(null);
          setSelectionBeforeRange(new Set());
        }
      } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'a') {
        const active = document.activeElement;
        const isInput = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement;
        if (!isInput && !itemsToDelete && data?.items && data.items.length > 0) {
          e.preventDefault();
          const allPaths = new Set(data.items.map((i) => i.path));
          setSelectedPaths(allPaths);
          setAnchorIndex(0);
          setSelectionBeforeRange(allPaths);
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [itemsToDelete, selectedPaths, data]);

  const handleNavigate = (path: string) => {
    setSelectedPaths(new Set());
    setAnchorIndex(null);
    setSelectionBeforeRange(new Set());
    loadFiles(path);
  };

  const handleToggleSelectAll = () => {
    if (!data?.items || data.items.length === 0) return;
    if (allSelected) {
      setSelectedPaths(new Set());
      setAnchorIndex(null);
      setSelectionBeforeRange(new Set());
    } else {
      const allPaths = new Set(data.items.map((i) => i.path));
      setSelectedPaths(allPaths);
      setAnchorIndex(0);
      setSelectionBeforeRange(allPaths);
    }
  };

  const handleSelect = (index: number, item: FileItem, shiftKey: boolean) => {
    if (!data?.items) return;

    if (shiftKey) {
      // Prevent browser text selection on shift-click
      window.getSelection()?.removeAllRanges();

      const anchor = anchorIndex !== null ? anchorIndex : 0;
      const start = Math.min(anchor, index);
      const end = Math.max(anchor, index);

      // Reconstruct range starting from baseline before range selection
      const next = new Set(selectionBeforeRange);
      for (let i = start; i <= end; i++) {
        const target = data.items[i];
        if (target) {
          next.add(target.path);
        }
      }

      if (anchorIndex === null) {
        setAnchorIndex(0);
        setSelectionBeforeRange(new Set());
      }

      setSelectedPaths(next);
    } else {
      // Individual toggle
      const next = new Set(selectedPaths);
      if (next.has(item.path)) {
        next.delete(item.path);
      } else {
        next.add(item.path);
      }
      setSelectedPaths(next);
      setAnchorIndex(index);
      setSelectionBeforeRange(next);
    }
  };

  const handleRowClick = (index: number, item: FileItem, e: React.MouseEvent) => {
    const target = e.target as HTMLElement;
    // Don't toggle selection if clicking on buttons or links
    if (target.closest('button') || target.closest('a')) {
      return;
    }
    handleSelect(index, item, e.shiftKey);
  };

  const handleClearSelection = () => {
    setSelectedPaths(new Set());
    setAnchorIndex(null);
    setSelectionBeforeRange(new Set());
  };

  const handleDownload = (item: FileItem) => {
    const url = `/api/files/download?path=${encodeURIComponent(item.path)}`;
    window.open(url, '_blank');
  };

  const handleDownloadSelected = () => {
    const filesToDownload = data?.items?.filter(
      (item) => selectedPaths.has(item.path) && !item.is_dir
    ) || [];
    filesToDownload.forEach((file, idx) => {
      setTimeout(() => {
        handleDownload(file);
      }, idx * 150);
    });
  };

  const handleDeleteSelected = () => {
    const items = data?.items?.filter((item) => selectedPaths.has(item.path)) || [];
    if (items.length > 0) {
      setItemsToDelete(items);
    }
  };

  const handleConfirmDelete = async () => {
    if (!itemsToDelete || itemsToDelete.length === 0) return;
    setDeleteLoading(true);
    setError(null);

    try {
      const errorMessages: string[] = [];
      for (const item of itemsToDelete) {
        const res = await fetch(`/api/files?path=${encodeURIComponent(item.path)}`, {
          method: 'DELETE',
        });
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          errorMessages.push(errData.error || `Failed to delete ${item.name}`);
        }
      }
      if (errorMessages.length > 0) {
        throw new Error(errorMessages.join(', '));
      }
      setItemsToDelete(null);
      setSelectedPaths(new Set());
      setAnchorIndex(null);
      setSelectionBeforeRange(new Set());
      loadFiles(currentPath);
      onFileDeleted?.();
    } catch (err: any) {
      setError(err.message);
    } finally {
      setDeleteLoading(false);
    }
  };

  const selectedItems = data?.items?.filter((item) => selectedPaths.has(item.path)) || [];
  const selectedFiles = selectedItems.filter((item) => !item.is_dir);
  const selectedTotalSize = selectedFiles.reduce((acc, item) => acc + (item.size || 0), 0);

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

      {/* Multi-selection action toolbar */}
      {selectedCount > 0 && (
        <div className="flex items-center justify-between gap-4 px-4 py-2.5 bg-slate-900 border border-sky-500/30 rounded-xl text-xs">
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-2">
              <span className="inline-flex items-center justify-center px-2 py-0.5 rounded-md bg-sky-500/10 text-sky-400 font-semibold font-mono text-[11px] border border-sky-500/20">
                {selectedCount}
              </span>
              <span className="text-slate-200 font-medium">
                {selectedCount === 1 ? 'item selected' : 'items selected'}
              </span>
            </div>

            {selectedTotalSize > 0 && (
              <span className="text-slate-400 font-mono hidden sm:inline">
                ({formatBytes(selectedTotalSize)})
              </span>
            )}
          </div>

          <div className="flex items-center gap-2">
            {selectedFiles.length > 0 && (
              <button
                onClick={handleDownloadSelected}
                aria-label={`Download ${selectedFiles.length} selected ${selectedFiles.length === 1 ? 'file' : 'files'}`}
                className="flex items-center gap-1.5 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg transition font-medium cursor-pointer"
                title={selectedFiles.length === 1 ? 'Download selected file' : `Download ${selectedFiles.length} selected files`}
              >
                <Download className="w-3.5 h-3.5 text-sky-400" />
                <span>Download{selectedFiles.length > 1 ? ` (${selectedFiles.length})` : ''}</span>
              </button>
            )}

            <button
              onClick={handleDeleteSelected}
              aria-label={`Delete ${selectedCount} selected ${selectedCount === 1 ? 'item' : 'items'}`}
              className="flex items-center gap-1.5 px-3 py-1.5 bg-rose-500/10 hover:bg-rose-500/20 border border-rose-500/30 text-rose-400 rounded-lg transition font-medium cursor-pointer"
              title="Delete selected items"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>Delete ({selectedCount})</span>
            </button>

            <button
              onClick={handleClearSelection}
              aria-label="Clear selection"
              className="flex items-center gap-1 px-2.5 py-1.5 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded-lg transition text-xs cursor-pointer"
              title="Clear selection (Esc)"
            >
              <X className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Clear</span>
            </button>
          </div>
        </div>
      )}

      {/* Error display */}
      {error && (
        <div className="p-4 bg-rose-500/10 border border-rose-500/30 rounded-xl text-xs text-rose-400">
          {error}
        </div>
      )}

      {/* File Table */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        <table className="w-full text-left text-xs" aria-multiselectable="true">
          <thead className="bg-slate-900/80 border-b border-slate-800 text-slate-400 uppercase tracking-wider font-semibold">
            <tr>
              <th className="py-3 px-3 w-10 text-center">
                <input
                  type="checkbox"
                  ref={headerCheckboxRef}
                  checked={allSelected}
                  onChange={handleToggleSelectAll}
                  aria-label={allSelected ? 'Deselect all items' : 'Select all items'}
                  className="w-4 h-4 rounded border-slate-700 bg-slate-950 text-sky-500 accent-sky-500 focus:ring-sky-500/30 focus:ring-offset-0 focus:ring-2 cursor-pointer transition-colors"
                />
              </th>
              <th className="py-3 px-4">Name</th>
              <th className="py-3 px-4 w-28 text-right hidden sm:table-cell">Size</th>
              <th className="py-3 px-4 w-44 text-right hidden md:table-cell">Modified</th>
              <th className="py-3 px-4 w-24 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800/60">
            {data && data.items && data.items.length > 0 ? (
              data.items.map((item, index) => {
                const isSelected = selectedPaths.has(item.path);
                return (
                  <tr
                    key={item.path}
                    onClick={(e) => handleRowClick(index, item, e)}
                    aria-selected={isSelected}
                    className={`transition group cursor-pointer select-none ${
                      isSelected
                        ? 'bg-sky-500/10 hover:bg-sky-500/[0.14]'
                        : 'hover:bg-slate-800/40'
                    }`}
                  >
                    {/* Checkbox */}
                    <td className="py-3 px-3 w-10 text-center" onClick={(e) => e.stopPropagation()}>
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => {}}
                        onClick={(e) => handleSelect(index, item, e.shiftKey)}
                        aria-label={`Select ${item.name}`}
                        className="w-4 h-4 rounded border-slate-700 bg-slate-950 text-sky-500 accent-sky-500 focus:ring-sky-500/30 focus:ring-offset-0 focus:ring-2 cursor-pointer transition-colors"
                      />
                    </td>

                    {/* Name + Icon */}
                    <td className="py-3 px-4">
                      {item.is_dir ? (
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            handleNavigate(item.path);
                          }}
                          className="flex items-center gap-2.5 text-sky-300 hover:text-sky-200 font-medium text-left cursor-pointer"
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
                      <div className="flex items-center justify-end gap-1" onClick={(e) => e.stopPropagation()}>
                        {!item.is_dir && (
                          <button
                            onClick={() => handleDownload(item)}
                            aria-label={`Download file ${item.name}`}
                            className="p-1.5 text-slate-400 hover:text-sky-300 hover:bg-sky-500/10 rounded transition cursor-pointer"
                            title="Download file"
                          >
                            <Download className="w-3.5 h-3.5" />
                          </button>
                        )}
                        <button
                          onClick={() => setItemsToDelete([item])}
                          aria-label={`Delete ${item.is_dir ? 'folder' : 'file'} ${item.name}`}
                          className="p-1.5 text-slate-400 hover:text-rose-400 hover:bg-rose-500/10 rounded transition cursor-pointer"
                          title={item.is_dir ? 'Delete folder' : 'Delete file'}
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })
            ) : (
              <tr>
                <td colSpan={5} className="py-14 text-center">
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
      {itemsToDelete && (
        <div
          onClick={(e) => {
            if (e.target === e.currentTarget) setItemsToDelete(null);
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
              <div className="flex-1 min-w-0">
                <h3 id="delete-dialog-title" className="text-base font-semibold text-white">
                  {itemsToDelete.length === 1
                    ? `Delete ${itemsToDelete[0].is_dir ? 'Folder' : 'File'}`
                    : `Delete ${itemsToDelete.length} Items`}
                </h3>
                <p className="text-xs text-slate-400 mt-1">
                  {itemsToDelete.length === 1 ? (
                    <>
                      Are you sure you want to permanently delete{' '}
                      <strong className="text-slate-200">"{itemsToDelete[0].name}"</strong>
                      {itemsToDelete[0].is_dir ? ' and all of its contents' : ''}? This action cannot be undone.
                    </>
                  ) : (
                    <>
                      Are you sure you want to permanently delete these{' '}
                      <strong className="text-slate-200">{itemsToDelete.length} items</strong> and all of their contents? This action cannot be undone.
                    </>
                  )}
                </p>

                {itemsToDelete.length > 1 && (
                  <div className="mt-3 max-h-36 overflow-y-auto bg-slate-950/60 rounded-lg p-2.5 border border-slate-800/80 divide-y divide-slate-800/40 text-xs text-slate-300">
                    {itemsToDelete.slice(0, 5).map((item) => (
                      <div key={item.path} className="py-1 flex items-center gap-2 truncate">
                        {item.is_dir ? (
                          <Folder className="w-3.5 h-3.5 text-sky-400 shrink-0 fill-sky-400/20" />
                        ) : (
                          <File className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                        )}
                        <span className="truncate">{item.name}</span>
                      </div>
                    ))}
                    {itemsToDelete.length > 5 && (
                      <div className="pt-1.5 text-[11px] text-slate-500 italic">
                        + {itemsToDelete.length - 5} more items
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>

            <div className="flex items-center justify-end gap-2 pt-2">
              <button
                type="button"
                disabled={deleteLoading}
                onClick={() => setItemsToDelete(null)}
                className="px-3 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium rounded-lg transition cursor-pointer"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={deleteLoading}
                onClick={handleConfirmDelete}
                className="px-4 py-2 bg-rose-600 hover:bg-rose-500 text-white text-xs font-semibold rounded-lg transition disabled:opacity-50 flex items-center gap-1.5 cursor-pointer"
              >
                {deleteLoading
                  ? 'Deleting...'
                  : itemsToDelete.length > 1
                  ? `Delete ${itemsToDelete.length} Items`
                  : 'Delete'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
