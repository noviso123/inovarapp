import React from 'react';
import { Snowflake } from 'lucide-react';

interface LogoProps {
  className?: string;
  invertido?: boolean;
  size?: 'sm' | 'md' | 'lg';
}

export function Logo({ className = '', invertido = true, size = 'md' }: LogoProps) {
  const iconSizes = {
    sm: 'w-5 h-5',
    md: 'w-7 h-7',
    lg: 'w-9 h-9',
  };

  const titleSizes = {
    sm: 'text-base',
    md: 'text-lg',
    lg: 'text-2xl',
  };

  const subSizes = {
    sm: 'text-[9px] tracking-[0.18em]',
    md: 'text-[11px] tracking-[0.2em]',
    lg: 'text-xs tracking-[0.25em]',
  };

  return (
    <div className={`flex items-center gap-2 select-none ${className}`}>
      <div className="relative flex items-center justify-center p-1.5 rounded-xl bg-inovar-yellow/15 border border-inovar-yellow/30 shadow-inner">
        <Snowflake className={`${iconSizes[size]} text-inovar-yellow`} />
      </div>
      <div className="leading-none">
        <span className={`block font-extrabold tracking-wider ${titleSizes[size]} ${invertido ? 'text-white' : 'text-inovar-navy'}`}>
          INOVAR
        </span>
        <span className={`block font-bold ${subSizes[size]} text-inovar-yellow`}>
          REFRIGERAÇÃO
        </span>
      </div>
    </div>
  );
}

export default Logo;
