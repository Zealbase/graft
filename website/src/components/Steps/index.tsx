/**
 * Steps / Step — vertical timeline stepper component.
 *
 * Usage in MDX (no import needed — registered via MDXComponents.tsx):
 *
 *   <Steps>
 *     <Step title="Install">
 *       ```bash
 *       go install ...
 *       ```
 *     </Step>
 *     <Step title="Initialize">
 *       Run inside your git repo: `graft init`
 *     </Step>
 *   </Steps>
 *
 * Styled via co-located styles.module.css using gx-* class names.
 */
import React from 'react';
import styles from './styles.module.css';

interface StepProps {
  title: string;
  children: React.ReactNode;
}

export function Step({ title, children }: StepProps) {
  return (
    <div className={styles.step}>
      <div className={styles.stepNumber} />
      <div className={styles.stepContent}>
        <h3 className={styles.stepTitle}>{title}</h3>
        <div className={styles.stepBody}>{children}</div>
      </div>
    </div>
  );
}

interface StepsProps {
  children: React.ReactNode;
}

export function Steps({ children }: StepsProps) {
  return <div className={styles.steps}>{children}</div>;
}
