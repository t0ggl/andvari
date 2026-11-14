import React, { useEffect, useState, useRef, useCallback } from 'react';
import {
  Folder,
  File,
  Download,
  Trash2,
  ChevronRight,
  Home,
  RefreshCw,
  AlertTriangle,
  X,
  Upload,
  FolderUp,
  FileUp,
  CheckCircle2,
  AlertCircle,
  Loader2,
  ChevronDown,
  ChevronUp,
  FolderPlus,
  Pencil,
  Archive,
} from 'lucide-react';
import { ExplorerResult, FileItem } from '../types';

interface Props {
  effectiveUsername: string;
  onFileDeleted?: () => void;
  onFileUploaded?: () => void;
}

export interface UploadTask {
  id: string;
  name: string;
  relativePath: string;
  size: number;
  isDir?: boolean;
  file?: File;
  status: 'pending' | 'uploading' | 'success' | 'error';
  progress: number;
  error?: string;
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

export const FileExplorer: React.FC<Props> = ({ effectiveUsername, onFileDeleted, onFileUploaded }) => {
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

  // New folder modal state
  const [isNewFolderOpen, setIsNewFolderOpen] = useState(false);
  const [newFolderName, setNewFolderName] = useState('');
  const [newFolderLoading, setNewFolderLoading] = useState(false);
  const [newFolderError, setNewFolderError] = useState<string | null>(null);
  const newFolderInputRef = useRef<HTMLInputElement | null>(null);

  // Rename modal state
  const [itemToRename, setItemToRename] = useState<FileItem | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const [renameLoading, setRenameLoading] = useState(false);
  const [renameError, setRenameError] = useState<string | null>(null);
  const renameInputRef = useRef<HTMLInputElement | null>(null);

  // Upload state
  const [uploadTasks, setUploadTasks] = useState<UploadTask[]>([]);
  const [isUploadWidgetOpen, setIsUploadWidgetOpen] = useState(false);
  const [isUploadWidgetMinimized, setIsUploadWidgetMinimized] = useState(false);
  const [isUploadMenuOpen, setIsUploadMenuOpen] = useState(false);
  const [isDraggingOver, setIsDraggingOver] = useState(false);

  const headerCheckboxRef = useRef<HTMLInputElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const folderInputRef = useRef<HTMLInputElement | null>(null);
  const uploadMenuRef = useRef<HTMLDivElement | null>(null);
  const dragCounter = useRef(0);

  const loadFiles = useCallback(async (path: string) => {
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
  }, []);

  useEffect(() => {
    loadFiles(currentPath);
  }, [effectiveUsername, loadFiles]);

  // Close upload dropdown when clicking outside
  useEffect(() => {
    const handleOutsideClick = (e: MouseEvent) => {
      if (uploadMenuRef.current && !uploadMenuRef.current.contains(e.target as Node)) {
        setIsUploadMenuOpen(false);
      }
    };
    document.addEventListener('mousedown', handleOutsideClick);
    return () => document.removeEventListener('mousedown', handleOutsideClick);
  }, []);

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

  // Auto-focus New Folder input
  useEffect(() => {
    if (isNewFolderOpen && newFolderInputRef.current) {
      newFolderInputRef.current.focus();
      newFolderInputRef.current.select();
    }
  }, [isNewFolderOpen]);

  // Auto-focus Rename input and select filename
  useEffect(() => {
    if (itemToRename && renameInputRef.current) {
      renameInputRef.current.focus();
      const lastDot = itemToRename.name.lastIndexOf('.');
      if (!itemToRename.is_dir && lastDot > 0) {
        renameInputRef.current.setSelectionRange(0, lastDot);
      } else {
        renameInputRef.current.select();
      }
    }
  }, [itemToRename]);

  // Global keydown handler for Escape, F2, and Ctrl+A / Cmd+A
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (itemsToDelete) {
          setItemsToDelete(null);
        } else if (isNewFolderOpen) {
          setIsNewFolderOpen(false);
        } else if (itemToRename) {
          setItemToRename(null);
        } else if (isUploadMenuOpen) {
          setIsUploadMenuOpen(false);
        } else if (selectedPaths.size > 0) {
          setSelectedPaths(new Set());
          setAnchorIndex(null);
          setSelectionBeforeRange(new Set());
        }
      } else if (e.key === 'F2') {
        const active = document.activeElement;
        const isInput = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement;
        if (!isInput && !itemsToDelete && !isNewFolderOpen && !itemToRename && selectedPaths.size === 1) {
          e.preventDefault();
          const selectedPath = Array.from(selectedPaths)[0];
          const item = data?.items?.find((i) => i.path === selectedPath);
          if (item) {
            setItemToRename(item);
            setRenameValue(item.name);
            setRenameError(null);
          }
        }
      } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'a') {
        const active = document.activeElement;
        const isInput = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement;
        if (!isInput && !itemsToDelete && !isNewFolderOpen && !itemToRename && data?.items && data.items.length > 0) {
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
  }, [itemsToDelete, isNewFolderOpen, itemToRename, selectedPaths, data, isUploadMenuOpen]);

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
      window.getSelection()?.removeAllRanges();

      const anchor = anchorIndex !== null ? anchorIndex : 0;
      const start = Math.min(anchor, index);
      const end = Math.max(anchor, index);

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

  const handleDownloadArchive = (paths: string[], filename?: string) => {
    const params = new URLSearchParams();
    paths.forEach((p) => params.append('paths', p));
    if (filename) {
      params.set('filename', filename);
    }
    const url = `/api/files/archive?${params.toString()}`;
    const link = document.createElement('a');
    link.href = url;
    link.download = filename || 'archive.zip';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  const handleDownloadZip = () => {
    const items = data?.items?.filter((item) => selectedPaths.has(item.path)) || [];
    if (items.length === 0) return;
    const filename = items.length === 1 && items[0].is_dir ? `${items[0].name}.zip` : 'archive.zip';
    handleDownloadArchive(items.map((i) => i.path), filename);
  };

  const handleDownloadFolderZip = (item: FileItem) => {
    handleDownloadArchive([item.path], `${item.name}.zip`);
  };

  const handleStartRename = (item: FileItem) => {
    setItemToRename(item);
    setRenameValue(item.name);
    setRenameError(null);
  };

  const handleCreateFolderSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = newFolderName.trim();
    if (!trimmed) {
      setNewFolderError('Folder name cannot be empty');
      return;
    }
    if (trimmed.includes('/') || trimmed.includes('\\')) {
      setNewFolderError('Folder name cannot contain slashes');
      return;
    }

    setNewFolderLoading(true);
    setNewFolderError(null);

    const folderPath = currentPath ? `${currentPath}/${trimmed}` : trimmed;
    try {
      const res = await fetch(`/api/files/folder?path=${encodeURIComponent(folderPath)}`, {
        method: 'POST',
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || 'Failed to create folder');
      }
      setIsNewFolderOpen(false);
      setNewFolderName('');
      loadFiles(currentPath);
    } catch (err: any) {
      setNewFolderError(err.message);
    } finally {
      setNewFolderLoading(false);
    }
  };

  const handleRenameSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!itemToRename) return;
    const trimmed = renameValue.trim();
    if (!trimmed) {
      setRenameError('Name cannot be empty');
      return;
    }
    if (trimmed.includes('/') || trimmed.includes('\\')) {
      setRenameError('Name cannot contain slashes');
      return;
    }
    if (trimmed === itemToRename.name) {
      setItemToRename(null);
      return;
    }

    setRenameLoading(true);
    setRenameError(null);

    const newPath = currentPath ? `${currentPath}/${trimmed}` : trimmed;
    try {
      const res = await fetch('/api/files/rename', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          old_path: itemToRename.path,
          new_path: newPath,
        }),
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || 'Failed to rename item');
      }
      setItemToRename(null);
      setSelectedPaths((prev) => {
        if (prev.has(itemToRename.path)) {
          const next = new Set(prev);
          next.delete(itemToRename.path);
          next.add(newPath);
          return next;
        }
        return prev;
      });
      loadFiles(currentPath);
    } catch (err: any) {
      setRenameError(err.message);
    } finally {
      setRenameLoading(false);
    }
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

  // -------------------------------------------------------------
  // Upload Processing Pipeline
  // -------------------------------------------------------------
  const executeUploadTasks = async (tasksToRun: UploadTask[], targetPath: string) => {
    if (tasksToRun.length === 0) return;

    setIsUploadWidgetOpen(true);
    setIsUploadWidgetMinimized(false);

    // Concurrency pool of 2 workers
    const concurrency = 2;
    let taskIndex = 0;
    let anySuccess = false;

    const runWorker = async () => {
      while (taskIndex < tasksToRun.length) {
        const currentTask = tasksToRun[taskIndex++];
        if (!currentTask) break;

        // Mark task as uploading
        setUploadTasks((prev) =>
          prev.map((t) => (t.id === currentTask.id ? { ...t, status: 'uploading', progress: 0 } : t))
        );

        if (currentTask.isDir) {
          // Empty folder upload
          const folderCombined = targetPath
            ? `${targetPath}/${currentTask.relativePath}`
            : currentTask.relativePath;
          try {
            const res = await fetch(
              `/api/files/upload?path=${encodeURIComponent(folderCombined)}&is_dir=true`,
              { method: 'POST' }
            );
            if (!res.ok) {
              const errBody = await res.json().catch(() => ({}));
              throw new Error(errBody.error || `Directory creation failed: ${res.statusText}`);
            }
            anySuccess = true;
            setUploadTasks((prev) =>
              prev.map((t) =>
                t.id === currentTask.id ? { ...t, status: 'success', progress: 100 } : t
              )
            );
          } catch (err: any) {
            setUploadTasks((prev) =>
              prev.map((t) =>
                t.id === currentTask.id
                  ? { ...t, status: 'error', error: err.message || 'Creation failed' }
                  : t
              )
            );
          }
        } else if (currentTask.file) {
          // File upload via XMLHttpRequest for streaming progress
          await new Promise<void>((resolve) => {
            const formData = new FormData();
            formData.append('files', currentTask.file!, currentTask.relativePath);

            const xhr = new XMLHttpRequest();
            xhr.open('POST', `/api/files/upload?path=${encodeURIComponent(targetPath)}`);

            xhr.upload.onprogress = (e) => {
              if (e.lengthComputable) {
                const percent = Math.min(100, Math.round((e.loaded / e.total) * 100));
                setUploadTasks((prev) =>
                  prev.map((t) =>
                    t.id === currentTask.id ? { ...t, progress: percent } : t
                  )
                );
              }
            };

            xhr.onload = () => {
              if (xhr.status >= 200 && xhr.status < 300) {
                anySuccess = true;
                setUploadTasks((prev) =>
                  prev.map((t) =>
                    t.id === currentTask.id ? { ...t, status: 'success', progress: 100 } : t
                  )
                );
              } else {
                let errorMsg = `Upload failed (${xhr.status})`;
                try {
                  const parsed = JSON.parse(xhr.responseText);
                  if (parsed.error) errorMsg = parsed.error;
                } catch {
                  if (xhr.status === 507) errorMsg = 'Quota exceeded: storage full';
                }
                setUploadTasks((prev) =>
                  prev.map((t) =>
                    t.id === currentTask.id
                      ? { ...t, status: 'error', error: errorMsg }
                      : t
                  )
                );
              }
              resolve();
            };

            xhr.onerror = () => {
              setUploadTasks((prev) =>
                prev.map((t) =>
                  t.id === currentTask.id
                    ? { ...t, status: 'error', error: 'Network connection failed' }
                    : t
                )
              );
              resolve();
            };

            xhr.send(formData);
          });
        }
      }
    };

    const workers = Array.from({ length: Math.min(concurrency, tasksToRun.length) }, () =>
      runWorker()
    );
    await Promise.all(workers);

    if (anySuccess) {
      loadFiles(currentPath);
      onFileUploaded?.();
    }
  };

  const queueUploads = (newTasks: UploadTask[]) => {
    if (newTasks.length === 0) return;
    setUploadTasks((prev) => [...prev, ...newTasks]);
    executeUploadTasks(newTasks, currentPath);
  };

  // Standard file input selection
  const handleFileInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || []);
    if (files.length === 0) return;

    const newTasks: UploadTask[] = files.map((f) => ({
      id: `${f.name}-${Date.now()}-${Math.random()}`,
      name: f.name,
      relativePath: f.name,
      size: f.size,
      file: f,
      status: 'pending',
      progress: 0,
    }));

    if (fileInputRef.current) fileInputRef.current.value = '';
    setIsUploadMenuOpen(false);
    queueUploads(newTasks);
  };

  // Folder input selection (webkitdirectory)
  const handleFolderInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || []);
    if (files.length === 0) return;

    const newTasks: UploadTask[] = files.map((f) => {
      const relPath = (f as any).webkitRelativePath || f.name;
      return {
        id: `${relPath}-${Date.now()}-${Math.random()}`,
        name: f.name,
        relativePath: relPath,
        size: f.size,
        file: f,
        status: 'pending',
        progress: 0,
      };
    });

    if (folderInputRef.current) folderInputRef.current.value = '';
    setIsUploadMenuOpen(false);
    queueUploads(newTasks);
  };

  // Drag and drop with recursive directory scanning
  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current += 1;
    if (e.dataTransfer.types && Array.from(e.dataTransfer.types).includes('Files')) {
      setIsDraggingOver(true);
    }
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = 'copy';
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current -= 1;
    if (dragCounter.current <= 0) {
      setIsDraggingOver(false);
      dragCounter.current = 0;
    }
  };

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDraggingOver(false);
    dragCounter.current = 0;

    const items = e.dataTransfer.items;
    if (!items || items.length === 0) {
      const files = Array.from(e.dataTransfer.files || []);
      if (files.length === 0) return;
      const tasks: UploadTask[] = files.map((f) => ({
        id: `${f.name}-${Date.now()}-${Math.random()}`,
        name: f.name,
        relativePath: f.name,
        size: f.size,
        file: f,
        status: 'pending',
        progress: 0,
      }));
      queueUploads(tasks);
      return;
    }

    const tasks: UploadTask[] = [];

    // Helper to read entries in chunks from FileSystemDirectoryReader
    const readAllDirEntries = async (dirReader: any): Promise<any[]> => {
      const entries: any[] = [];
      while (true) {
        const batch: any[] = await new Promise((resolve, reject) => {
          dirReader.readEntries(resolve, reject);
        });
        if (!batch || batch.length === 0) break;
        entries.push(...batch);
      }
      return entries;
    };

    const traverse = async (entry: any, basePath = ''): Promise<void> => {
      if (!entry) return;
      if (entry.isFile) {
        const file: File = await new Promise((resolve, reject) => {
          entry.file(resolve, reject);
        });
        const relPath = basePath ? `${basePath}/${file.name}` : file.name;
        tasks.push({
          id: `${relPath}-${Date.now()}-${Math.random()}`,
          name: file.name,
          relativePath: relPath,
          size: file.size,
          file,
          status: 'pending',
          progress: 0,
        });
      } else if (entry.isDirectory) {
        const dirRelPath = basePath ? `${basePath}/${entry.name}` : entry.name;
        const reader = entry.createReader();
        const children = await readAllDirEntries(reader);
        if (children.length === 0) {
          tasks.push({
            id: `${dirRelPath}-${Date.now()}-${Math.random()}`,
            name: entry.name,
            relativePath: dirRelPath,
            size: 0,
            isDir: true,
            status: 'pending',
            progress: 0,
          });
        } else {
          for (const child of children) {
            await traverse(child, dirRelPath);
          }
        }
      }
    };

    const promises: Promise<void>[] = [];
    for (let i = 0; i < items.length; i++) {
      const item = items[i];
      const entry = (item as any).webkitGetAsEntry ? (item as any).webkitGetAsEntry() : null;
      if (entry) {
        promises.push(traverse(entry));
      } else {
        const file = item.getAsFile();
        if (file) {
          tasks.push({
            id: `${file.name}-${Date.now()}-${Math.random()}`,
            name: file.name,
            relativePath: file.name,
            size: file.size,
            file,
            status: 'pending',
            progress: 0,
          });
        }
      }
    }

    await Promise.all(promises);
    if (tasks.length > 0) {
      queueUploads(tasks);
    }
  };

  const handleDismissUploadWidget = () => {
    setIsUploadWidgetOpen(false);
    setUploadTasks((prev) => prev.filter((t) => t.status === 'uploading'));
  };

  const handleClearFinishedUploads = () => {
    setUploadTasks((prev) => prev.filter((t) => t.status === 'uploading' || t.status === 'pending'));
    if (uploadTasks.every((t) => t.status !== 'uploading' && t.status !== 'pending')) {
      setIsUploadWidgetOpen(false);
    }
  };

  const selectedItems = data?.items?.filter((item) => selectedPaths.has(item.path)) || [];
  const selectedFiles = selectedItems.filter((item) => !item.is_dir);
  const selectedTotalSize = selectedFiles.reduce((acc, item) => acc + (item.size || 0), 0);
  const hasSelectedFolder = selectedItems.some((item) => item.is_dir);
  const isMultipleSelected = selectedItems.length > 1;

  // Upload widget summary metrics
  const activeUploads = uploadTasks.filter((t) => t.status === 'uploading' || t.status === 'pending');
  const errorUploads = uploadTasks.filter((t) => t.status === 'error');
  const completedUploads = uploadTasks.filter((t) => t.status === 'success');
  const totalUploads = uploadTasks.length;
  const isUploadInProgress = activeUploads.length > 0;
  const overallProgress =
    totalUploads > 0
      ? Math.round(
          uploadTasks.reduce((acc, t) => acc + (t.status === 'success' ? 100 : t.progress), 0) /
            totalUploads
        )
      : 0;

  return (
    <div
      className="relative space-y-4"
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {/* Hidden file inputs */}
      <input
        type="file"
        ref={fileInputRef}
        onChange={handleFileInputChange}
        style={{ display: 'none' }}
        multiple
      />
      <input
        type="file"
        ref={folderInputRef}
        onChange={handleFolderInputChange}
        style={{ display: 'none' }}
        {...({ webkitdirectory: '', directory: '' } as any)}
        multiple
      />

      {/* Drag and Drop Visual Overlay */}
      {isDraggingOver && (
        <div className="absolute inset-0 z-40 rounded-xl border-2 border-dashed border-sky-400 bg-sky-950/80 backdrop-blur-sm flex flex-col items-center justify-center gap-3 p-6 text-center pointer-events-none animate-in fade-in duration-150">
          <div className="w-16 h-16 rounded-full bg-sky-500/20 border border-sky-400/40 flex items-center justify-center text-sky-400 shadow-lg shadow-sky-500/10">
            <Upload className="w-8 h-8 animate-bounce" />
          </div>
          <div className="space-y-1">
            <h3 className="text-sm font-semibold text-white">
              Drop files or folders to upload
            </h3>
            <p className="text-xs text-sky-300">
              Uploading to:{' '}
              <strong className="text-white font-mono">
                {currentPath ? `/${currentPath}` : 'Root'}
              </strong>
            </p>
            <p className="text-[11px] text-slate-400 mt-1">
              Supports recursive subdirectories and empty folders
            </p>
          </div>
        </div>
      )}

      {/* Top action bar: breadcrumbs, upload button & refresh */}
      <div className="flex items-center justify-between gap-3 p-3 bg-slate-900 border border-slate-800 rounded-xl">
        {/* Breadcrumbs */}
        <nav
          aria-label="Directory navigation"
          className="flex items-center gap-1.5 text-xs text-slate-300 overflow-x-auto py-1 min-w-0 flex-1"
        >
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

          {data?.breadcrumbs &&
            data.breadcrumbs.map((crumb, idx) => {
              const crumbPath = data.breadcrumbs.slice(0, idx + 1).join('/');
              const isLast = idx === data.breadcrumbs.length - 1;
              return (
                <React.Fragment key={crumbPath}>
                  <ChevronRight className="w-3.5 h-3.5 text-slate-600 flex-shrink-0" />
                  <button
                    onClick={() => handleNavigate(crumbPath)}
                    aria-label={`Navigate to ${crumb}`}
                    className={`px-2 py-1 rounded hover:bg-slate-800 transition truncate max-w-[120px] sm:max-w-none ${
                      isLast ? 'text-sky-400 font-semibold' : 'text-slate-400'
                    }`}
                  >
                    {crumb}
                  </button>
                </React.Fragment>
              );
            })}
        </nav>

        {/* Action Controls: New Folder, Upload & Refresh */}
        <div className="flex items-center gap-2 flex-shrink-0">
          <button
            onClick={() => {
              setNewFolderName('');
              setNewFolderError(null);
              setIsNewFolderOpen(true);
            }}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 hover:text-white rounded-lg transition text-xs font-semibold border border-slate-700/60 shadow-sm cursor-pointer"
            title="Create new folder"
          >
            <FolderPlus className="w-3.5 h-3.5 text-sky-400" />
            <span>New Folder</span>
          </button>

          {/* Upload Dropdown */}
          <div className="relative" ref={uploadMenuRef}>
            <button
              onClick={() => setIsUploadMenuOpen(!isUploadMenuOpen)}
              className="flex items-center gap-1.5 px-3 py-1.5 bg-sky-600 hover:bg-sky-500 text-white rounded-lg transition text-xs font-semibold shadow-sm cursor-pointer"
              title="Upload files or folders"
              aria-expanded={isUploadMenuOpen}
              aria-haspopup="true"
            >
              <Upload className="w-3.5 h-3.5" />
              <span>Upload</span>
              <ChevronDown className="w-3 h-3 opacity-70" />
            </button>

            {isUploadMenuOpen && (
              <div className="absolute right-0 mt-1.5 w-44 bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-1.5 z-50 text-xs animate-in fade-in slide-in-from-top-1 duration-150">
                <button
                  onClick={() => {
                    fileInputRef.current?.click();
                    setIsUploadMenuOpen(false);
                  }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-slate-200 hover:bg-slate-800 hover:text-white rounded-lg transition text-left cursor-pointer"
                >
                  <FileUp className="w-4 h-4 text-sky-400" />
                  <span>Upload Files</span>
                </button>
                <button
                  onClick={() => {
                    folderInputRef.current?.click();
                    setIsUploadMenuOpen(false);
                  }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-slate-200 hover:bg-slate-800 hover:text-white rounded-lg transition text-left cursor-pointer"
                >
                  <FolderUp className="w-4 h-4 text-sky-400" />
                  <span>Upload Folder</span>
                </button>
              </div>
            )}
          </div>

          {/* Refresh button */}
          <button
            onClick={() => loadFiles(currentPath)}
            disabled={loading}
            aria-label="Refresh directory contents"
            className="p-1.5 text-slate-400 hover:text-white hover:bg-slate-800 rounded-lg transition cursor-pointer"
            title="Refresh directory"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin text-sky-400' : ''}`} />
          </button>
        </div>
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
            {selectedItems.length > 0 && (hasSelectedFolder || isMultipleSelected ? (
              <button
                onClick={handleDownloadZip}
                aria-label={`Download ${selectedItems.length} selected items as ZIP`}
                className="flex items-center gap-1.5 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg transition font-medium cursor-pointer"
                title="Download selected items as ZIP archive"
              >
                <Archive className="w-3.5 h-3.5 text-sky-400" />
                <span>
                  Download ZIP{isMultipleSelected ? ` (${selectedItems.length})` : ''}
                </span>
              </button>
            ) : selectedFiles.length === 1 ? (
              <button
                onClick={() => handleDownload(selectedFiles[0])}
                aria-label="Download selected file"
                className="flex items-center gap-1.5 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg transition font-medium cursor-pointer"
                title="Download selected file"
              >
                <Download className="w-3.5 h-3.5 text-sky-400" />
                <span>Download</span>
              </button>
            ) : null)}

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

                    {/* Actions: Download, Rename & Delete */}
                    <td className="py-3 px-4 text-right">
                      <div
                        className="flex items-center justify-end gap-1"
                        onClick={(e) => e.stopPropagation()}
                      >
                        {item.is_dir ? (
                          <button
                            onClick={() => handleDownloadFolderZip(item)}
                            aria-label={`Download folder ${item.name} as ZIP`}
                            className="p-1.5 text-slate-400 hover:text-sky-300 hover:bg-sky-500/10 rounded transition cursor-pointer"
                            title="Download folder as ZIP"
                          >
                            <Download className="w-3.5 h-3.5" />
                          </button>
                        ) : (
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
                          onClick={() => handleStartRename(item)}
                          aria-label={`Rename ${item.is_dir ? 'folder' : 'file'} ${item.name}`}
                          className="p-1.5 text-slate-400 hover:text-sky-300 hover:bg-sky-500/10 rounded transition cursor-pointer"
                          title={`Rename ${item.is_dir ? 'folder' : 'file'} (F2)`}
                        >
                          <Pencil className="w-3.5 h-3.5" />
                        </button>
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
                  <div className="flex flex-col items-center justify-center gap-3 max-w-sm mx-auto text-slate-500">
                    <div className="w-12 h-12 rounded-full bg-slate-800/80 flex items-center justify-center text-slate-400 mb-1 border border-slate-700/60">
                      <Folder className="w-6 h-6" />
                    </div>
                    <div className="space-y-1">
                      <span className="text-sm font-semibold text-slate-300 block">
                        {loading ? 'Loading files...' : 'Folder is empty'}
                      </span>
                      {!loading && (
                        <p className="text-xs text-slate-400">
                          Drag and drop files or folders here, or click Upload above.
                        </p>
                      )}
                    </div>

                    {!loading && (
                      <div className="flex items-center gap-2 pt-2">
                        <button
                          onClick={() => fileInputRef.current?.click()}
                          className="flex items-center gap-1.5 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-medium rounded-lg transition cursor-pointer"
                        >
                          <FileUp className="w-3.5 h-3.5 text-sky-400" />
                          <span>Select Files</span>
                        </button>
                        <button
                          onClick={() => folderInputRef.current?.click()}
                          className="flex items-center gap-1.5 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-medium rounded-lg transition cursor-pointer"
                        >
                          <FolderUp className="w-3.5 h-3.5 text-sky-400" />
                          <span>Select Folder</span>
                        </button>
                      </div>
                    )}
                  </div>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Floating Upload Progress Drawer */}
      {isUploadWidgetOpen && uploadTasks.length > 0 && (
        <div className="fixed bottom-4 right-4 z-50 w-96 max-w-[calc(100vw-2rem)] bg-slate-900 border border-slate-800 rounded-xl shadow-2xl backdrop-blur-md overflow-hidden text-xs transition-all animate-in slide-in-from-bottom-2 duration-200">
          {/* Header */}
          <div className="flex items-center justify-between px-3.5 py-2.5 bg-slate-900/90 border-b border-slate-800">
            <div className="flex items-center gap-2 min-w-0">
              {isUploadInProgress ? (
                <Loader2 className="w-4 h-4 text-sky-400 animate-spin flex-shrink-0" />
              ) : errorUploads.length > 0 ? (
                <AlertCircle className="w-4 h-4 text-rose-400 flex-shrink-0" />
              ) : (
                <CheckCircle2 className="w-4 h-4 text-emerald-400 flex-shrink-0" />
              )}
              <span className="font-semibold text-slate-200 truncate">
                {isUploadInProgress
                  ? `Uploading ${completedUploads.length + 1} of ${totalUploads}...`
                  : errorUploads.length > 0
                  ? `Upload finished with ${errorUploads.length} ${
                      errorUploads.length === 1 ? 'error' : 'errors'
                    }`
                  : `Uploaded ${totalUploads} ${totalUploads === 1 ? 'item' : 'items'}`}
              </span>
            </div>

            <div className="flex items-center gap-1 flex-shrink-0">
              <button
                onClick={() => setIsUploadWidgetMinimized(!isUploadWidgetMinimized)}
                aria-label={isUploadWidgetMinimized ? 'Expand upload panel' : 'Collapse upload panel'}
                className="p-1 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded transition cursor-pointer"
                title={isUploadWidgetMinimized ? 'Expand' : 'Collapse'}
              >
                {isUploadWidgetMinimized ? (
                  <ChevronUp className="w-3.5 h-3.5" />
                ) : (
                  <ChevronDown className="w-3.5 h-3.5" />
                )}
              </button>
              <button
                onClick={handleDismissUploadWidget}
                aria-label="Close upload panel"
                className="p-1 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded transition cursor-pointer"
                title="Close"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>

          {/* Progress bar */}
          <div className="w-full h-1 bg-slate-800 overflow-hidden">
            <div
              className={`h-full transition-all duration-200 ${
                errorUploads.length > 0 && !isUploadInProgress
                  ? 'bg-rose-500'
                  : 'bg-sky-500'
              }`}
              style={{ width: `${overallProgress}%` }}
            />
          </div>

          {/* Task list (collapsible) */}
          {!isUploadWidgetMinimized && (
            <div>
              <div className="max-h-60 overflow-y-auto divide-y divide-slate-800/50 p-1">
                {uploadTasks.map((task) => (
                  <div key={task.id} className="py-2 px-2.5 flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2 min-w-0 flex-1">
                      {task.isDir ? (
                        <Folder className="w-3.5 h-3.5 text-sky-400 flex-shrink-0 fill-sky-400/20" />
                      ) : (
                        <File className="w-3.5 h-3.5 text-slate-400 flex-shrink-0" />
                      )}
                      <div className="min-w-0 flex-1">
                        <p className="text-slate-200 truncate font-mono text-[11px]" title={task.relativePath}>
                          {task.relativePath}
                        </p>
                        <p className="text-[10px] text-slate-400">
                          {task.isDir ? 'Folder' : formatBytes(task.size)}
                        </p>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 flex-shrink-0">
                      {task.status === 'pending' && (
                        <span className="text-[10px] text-slate-400">Pending</span>
                      )}
                      {task.status === 'uploading' && (
                        <span className="text-[10px] font-mono text-sky-400">
                          {task.progress}%
                        </span>
                      )}
                      {task.status === 'success' && (
                        <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
                      )}
                      {task.status === 'error' && (
                        <span className="text-[10px] text-rose-400 font-medium" title={task.error}>
                          {task.error?.includes('quota') ? 'Quota exceeded' : 'Failed'}
                        </span>
                      )}
                    </div>
                  </div>
                ))}
              </div>

              {!isUploadInProgress && (
                <div className="p-2 border-t border-slate-800 bg-slate-900/60 flex justify-end">
                  <button
                    onClick={handleClearFinishedUploads}
                    className="px-2.5 py-1 text-[11px] text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded transition cursor-pointer"
                  >
                    Clear finished
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      )}

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
                      {itemsToDelete[0].is_dir ? ' and all of its contents' : ''}? This action
                      cannot be undone.
                    </>
                  ) : (
                    <>
                      Are you sure you want to permanently delete these{' '}
                      <strong className="text-slate-200">{itemsToDelete.length} items</strong> and
                      all of their contents? This action cannot be undone.
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

      {/* New Folder Modal */}
      {isNewFolderOpen && (
        <div
          onClick={(e) => {
            if (e.target === e.currentTarget && !newFolderLoading) setIsNewFolderOpen(false);
          }}
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in"
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="new-folder-dialog-title"
            className="w-full max-w-md bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4"
          >
            <div className="flex items-start gap-3">
              <div className="p-2 bg-sky-500/10 text-sky-400 rounded-lg">
                <FolderPlus className="w-5 h-5 flex-shrink-0" />
              </div>
              <div className="flex-1 min-w-0">
                <h3 id="new-folder-dialog-title" className="text-base font-semibold text-white">
                  New Folder
                </h3>
                <p className="text-xs text-slate-400 mt-1">
                  Create a new folder in{' '}
                  <span className="text-slate-200 font-mono">
                    {currentPath ? `/${currentPath}` : '/'}
                  </span>
                </p>
              </div>
            </div>

            <form onSubmit={handleCreateFolderSubmit} className="space-y-4">
              <div>
                <input
                  type="text"
                  ref={newFolderInputRef}
                  value={newFolderName}
                  onChange={(e) => {
                    setNewFolderName(e.target.value);
                    setNewFolderError(null);
                  }}
                  placeholder="Folder name"
                  disabled={newFolderLoading}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-700 rounded-lg text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500"
                  autoFocus
                />
                {newFolderError && (
                  <p className="text-xs text-rose-400 mt-1.5 flex items-center gap-1">
                    <AlertCircle className="w-3.5 h-3.5 flex-shrink-0" />
                    <span>{newFolderError}</span>
                  </p>
                )}
              </div>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  disabled={newFolderLoading}
                  onClick={() => setIsNewFolderOpen(false)}
                  className="px-3 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium rounded-lg transition cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={newFolderLoading || !newFolderName.trim()}
                  className="px-4 py-2 bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold rounded-lg transition disabled:opacity-50 flex items-center gap-1.5 cursor-pointer shadow-sm"
                >
                  {newFolderLoading ? (
                    <>
                      <Loader2 className="w-3.5 h-3.5 animate-spin" />
                      <span>Creating...</span>
                    </>
                  ) : (
                    <span>Create</span>
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Rename Modal */}
      {itemToRename && (
        <div
          onClick={(e) => {
            if (e.target === e.currentTarget && !renameLoading) setItemToRename(null);
          }}
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in"
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="rename-dialog-title"
            className="w-full max-w-md bg-slate-900 border border-slate-800 rounded-xl shadow-2xl p-6 space-y-4"
          >
            <div className="flex items-start gap-3">
              <div className="p-2 bg-sky-500/10 text-sky-400 rounded-lg">
                <Pencil className="w-5 h-5 flex-shrink-0" />
              </div>
              <div className="flex-1 min-w-0">
                <h3 id="rename-dialog-title" className="text-base font-semibold text-white">
                  Rename {itemToRename.is_dir ? 'Folder' : 'File'}
                </h3>
                <p className="text-xs text-slate-400 mt-1">
                  Enter a new name for{' '}
                  <span className="text-slate-200 font-semibold">{itemToRename.name}</span>
                </p>
              </div>
            </div>

            <form onSubmit={handleRenameSubmit} className="space-y-4">
              <div>
                <input
                  type="text"
                  ref={renameInputRef}
                  value={renameValue}
                  onChange={(e) => {
                    setRenameValue(e.target.value);
                    setRenameError(null);
                  }}
                  placeholder="New name"
                  disabled={renameLoading}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-700 rounded-lg text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500 focus:ring-1 focus:ring-sky-500"
                  autoFocus
                />
                {renameError && (
                  <p className="text-xs text-rose-400 mt-1.5 flex items-center gap-1">
                    <AlertCircle className="w-3.5 h-3.5 flex-shrink-0" />
                    <span>{renameError}</span>
                  </p>
                )}
              </div>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  disabled={renameLoading}
                  onClick={() => setItemToRename(null)}
                  className="px-3 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium rounded-lg transition cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={
                    renameLoading || !renameValue.trim() || renameValue.trim() === itemToRename.name
                  }
                  className="px-4 py-2 bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold rounded-lg transition disabled:opacity-50 flex items-center gap-1.5 cursor-pointer shadow-sm"
                >
                  {renameLoading ? (
                    <>
                      <Loader2 className="w-3.5 h-3.5 animate-spin" />
                      <span>Renaming...</span>
                    </>
                  ) : (
                    <span>Rename</span>
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
