import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/run.dart';
import '../providers/project.dart';

/// Tracks all runs across all projects, built from engine events.
class RunsNotifier extends StateNotifier<List<RunInfo>> {
  RunsNotifier() : super([]);

  void processEvent(EngineEvent event) {
    switch (event.type) {
      case 'pipeline-start':
        final run = RunInfo(
          runId: event.runId ?? '',
          agentName: event.agent ?? '',
          goal: event.goal ?? '',
          projectId: '',
          projectName: '',
          status: 'running',
          startedAt: event.timestamp,
          steps: [],
        );
        state = [run, ...state];
      case 'step-start':
        _updateCurrentRun(event.runId, (run) {
          final steps = [...run.steps];
          steps.add(StepProgress(
            name: event.step ?? '',
            type: event.stepType ?? '',
            status: 'running',
            startedAt: event.timestamp,
          ));
          return _copyRun(run, steps: steps);
        });
      case 'step-end':
        _updateCurrentRun(event.runId, (run) {
          final steps = run.steps.map((s) {
            if (s.name == event.step && s.status == 'running') {
              return StepProgress(
                name: s.name,
                type: s.type,
                status: event.status ?? 'success',
                duration: event.durationMs != null
                    ? Duration(milliseconds: event.durationMs!)
                    : null,
              );
            }
            return s;
          }).toList();
          return _copyRun(run, steps: steps);
        });
      case 'pipeline-end':
        _updateCurrentRun(event.runId, (run) {
          return _copyRun(
            run,
            status: event.status == 'success' ? 'success' : 'error',
            duration: event.durationMs != null
                ? Duration(milliseconds: event.durationMs!)
                : null,
          );
        });
      default:
        break;
    }
  }

  void _updateCurrentRun(String? runId, RunInfo Function(RunInfo) updater) {
    if (runId == null) return;
    state = state.map((r) {
      if (r.runId == runId) return updater(r);
      return r;
    }).toList();
  }

  RunInfo _copyRun(
    RunInfo run, {
    String? status,
    Duration? duration,
    List<StepProgress>? steps,
    String? prUrl,
    String? branch,
    String? haltReason,
  }) {
    return RunInfo(
      runId: run.runId,
      agentName: run.agentName,
      goal: run.goal,
      projectId: run.projectId,
      projectName: run.projectName,
      status: status ?? run.status,
      startedAt: run.startedAt,
      duration: duration ?? run.duration,
      prUrl: prUrl ?? run.prUrl,
      branch: branch ?? run.branch,
      haltReason: haltReason ?? run.haltReason,
      steps: steps ?? run.steps,
    );
  }
}

final runsProvider =
    StateNotifierProvider<RunsNotifier, List<RunInfo>>((ref) {
  return RunsNotifier();
});

/// Active runs only (status == 'running').
final activeRunsProvider = Provider<List<RunInfo>>((ref) {
  final runs = ref.watch(runsProvider);
  return runs.where((r) => r.status == 'running').toList();
});

/// Stores engine events per run, keyed by runId.
class RunEventsNotifier extends StateNotifier<Map<String, List<EngineEvent>>> {
  RunEventsNotifier() : super({});

  void addEvent(EngineEvent event) {
    if (event.runId == null) return;
    final runId = event.runId!;
    final existing = state[runId] ?? [];
    state = {...state, runId: [...existing, event]};
  }

  List<EngineEvent> eventsForRun(String runId) => state[runId] ?? const [];
}

final runEventsProvider =
    StateNotifierProvider<RunEventsNotifier, Map<String, List<EngineEvent>>>((ref) {
  return RunEventsNotifier();
});

/// Reactive events for a specific run.
final runEventsFamily = Provider.family<List<EngineEvent>, String>((ref, runId) {
  final allEvents = ref.watch(runEventsProvider);
  return allEvents[runId] ?? const [];
});

/// Wires engine events from WebSocket into RunsNotifier and RunEventsNotifier.
/// Watch this provider to activate the connection.
final engineEventWiringProvider = Provider<void>((ref) {
  final projectState = ref.watch(projectStateProvider.notifier);
  final runsNotifier = ref.read(runsProvider.notifier);
  final eventsNotifier = ref.read(runEventsProvider.notifier);

  projectState.onEngineEvent = (data) {
    final event = EngineEvent.fromJson(data);
    runsNotifier.processEvent(event);
    eventsNotifier.addEvent(event);
  };

  ref.onDispose(() {
    projectState.onEngineEvent = null;
  });
});
