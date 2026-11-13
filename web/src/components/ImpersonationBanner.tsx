import React from 'react';
import { AlertTriangle, XCircle } from 'lucide-react';

interface Props {
  impersonatedUser: string;
  onExit: () => void;
}

export const ImpersonationBanner: React.FC<Props> = ({ impersonatedUser, onExit }) => {
  return (
    <div className="bg-amber-950/60 border-b border-amber-500/30 px-3 sm:px-4 py-2 sm:py-2.5 text-xs text-amber-200">
      <div className="max-w-7xl mx-auto flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 sm:gap-4">
        <div className="flex items-center gap-2 min-w-0">
          <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0" />
          <span className="leading-snug">
            You are currently impersonating <strong>{impersonatedUser}</strong>. All file operations and token management apply directly to their account.
          </span>
        </div>
        <button
          onClick={onExit}
          className="flex items-center justify-center gap-1 px-2.5 py-1 bg-amber-500/20 hover:bg-amber-500/30 border border-amber-500/30 rounded text-amber-200 font-medium transition shrink-0 self-start sm:self-auto cursor-pointer"
        >
          <XCircle className="w-3.5 h-3.5" />
          <span>Exit Impersonation</span>
        </button>
      </div>
    </div>
  );
};
