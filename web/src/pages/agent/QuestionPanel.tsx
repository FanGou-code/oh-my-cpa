import React from 'react';
import { Button, Checkbox, Input, Radio } from 'antd';
import { QuestionCircleOutlined } from '../../components/icons';
import { useI18n } from '../../i18n';
import { decideOperation, failureCode } from './api';
import { failureKey, isQuestionAnswered, operationQuestions } from './state';
import type { Operation, QuestionReply } from './state';
import styles from './AgentPage.module.css';

export interface QuestionPanelProps {
  operation: Operation;
  onDecided: (operation: Operation) => void;
}

/** Typed answers are bounded on the server at the same length. */
const MAX_TYPED_ANSWER_CHARS = 2000;

/**
 * The agent's `ask_question`, answered where the operator would otherwise type.
 *
 * It takes the composer's place rather than opening a dialog: a question is part of the
 * conversation, and the answer above it is often what the operator needs to read to reply. Each
 * question offers the agent's options - one or several - and always a typed answer as well, so an
 * option list that misses the operator's case never traps them. Skip declines every question at
 * once, and the agent is told it was declined.
 */
export function QuestionPanel({ operation, onDecided }: QuestionPanelProps) {
  const { t } = useI18n();
  const questions = React.useMemo(() => operationQuestions(operation), [operation]);
  const [replies, setReplies] = React.useState<QuestionReply[]>(() => questions.map(() => ({ selected: [], text: '' })));
  const [pendingAction, setPendingAction] = React.useState<'answer' | 'skip' | ''>('');
  const [error, setError] = React.useState('');
  const isAnswered = isQuestionAnswered(replies, questions.length);

  const updateReply = (index: number, change: Partial<QuestionReply>) => {
    setReplies(current => current.map((reply, position) => (position === index ? { ...reply, ...change } : reply)));
  };

  const decide = async (approve: boolean) => {
    if (pendingAction || (approve && !isAnswered)) return;
    setPendingAction(approve ? 'answer' : 'skip');
    setError('');
    try {
      const answers = replies.map(reply => ({ selected: reply.selected, text: reply.text.trim() }));
      onDecided(await decideOperation(operation.id, approve, approve ? { answer: { answers } } : {}));
    } catch (cause) {
      setError(failureCode(cause));
    } finally {
      setPendingAction('');
    }
  };

  return (
    <section className={styles['question']} data-testid="agent-question" aria-label={t('agent.question.title')}>
      <header className={styles['question-head']}>
        <QuestionCircleOutlined className={styles['question-icon']} aria-hidden="true" />
        <span>{t('agent.question.title')}</span>
      </header>
      <div className={styles['question-body']}>
        {questions.map((question, index) => {
          const reply = replies[index];
          const options = question.options ?? [];
          const describe = (label: string, description?: string) => (
            <span className={styles['question-option']}>
              <span>{label}</span>
              {description && <span className={styles['question-option-description']}>{description}</span>}
            </span>
          );
          return (
            <fieldset key={`${index}-${question.question}`} className={styles['question-item']}>
              <legend className={styles['question-text']}>
                {question.header && <span className={styles['question-chip']}>{question.header}</span>}
                {question.question}
              </legend>
              {options.length > 0 && (question.multi_select ? (
                <Checkbox.Group
                  className={styles['question-options']}
                  value={reply.selected}
                  onChange={values => updateReply(index, { selected: values.map(String) })}
                  options={options.map(option => ({ value: option.label, label: describe(option.label, option.description) }))}
                />
              ) : (
                <Radio.Group
                  className={styles['question-options']}
                  value={reply.selected[0]}
                  onChange={event => updateReply(index, { selected: [String(event.target.value)] })}
                  options={options.map(option => ({ value: option.label, label: describe(option.label, option.description) }))}
                />
              ))}
              <Input.TextArea
                aria-label={t(options.length > 0 ? 'agent.question.other' : 'agent.question.answer')}
                placeholder={t(options.length > 0 ? 'agent.question.other' : 'agent.question.answer')}
                autoSize={{ minRows: 1, maxRows: 4 }}
                maxLength={MAX_TYPED_ANSWER_CHARS}
                value={reply.text}
                onChange={event => updateReply(index, { text: event.target.value })}
              />
            </fieldset>
          );
        })}
      </div>
      {error && (
        <div className={styles['failure']} role="alert">
          <span>{t(failureKey(error))}</span>
          <code>{error}</code>
        </div>
      )}
      <div className={styles['operation-actions']}>
        <Button disabled={!!pendingAction} loading={pendingAction === 'skip'} onClick={() => void decide(false)}>{t('agent.question.skip')}</Button>
        <Button type="primary" disabled={!isAnswered || !!pendingAction} loading={pendingAction === 'answer'} onClick={() => void decide(true)}>
          {t('agent.question.submit')}
        </Button>
      </div>
    </section>
  );
}
