/**
 * Swizzle-WRAP of @theme/MDXComponents.
 * Registers Lucide icons and custom components globally in MDX.
 * Add icons or components here; they become available in all .md/.mdx files
 * without per-file imports.
 *
 * Pattern: spread original MDXComponents first, then add/override below.
 */
import React from 'react';
import MDXComponents from '@theme-original/MDXComponents';
import {
  Zap,
  Box,
  Download,
  Settings,
  FlaskConical,
  Terminal,
  GitBranch,
  Rocket,
  CheckCircle,
  ArrowRight,
  Info,
  AlertTriangle,
} from 'lucide-react';

export default {
  ...MDXComponents,
  // Lucide icons — use in MDX as <Zap size={18} /> etc.
  Zap,
  Box,
  Download,
  Settings,
  FlaskConical,
  Terminal,
  GitBranch,
  Rocket,
  CheckCircle,
  ArrowRight,
  Info,
  AlertTriangle,
} as typeof MDXComponents;
