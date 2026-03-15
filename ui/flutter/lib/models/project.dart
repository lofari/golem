class ProjectInfo {
  final String id;
  final String path;
  final String name;
  final String phase;

  ProjectInfo({
    required this.id,
    required this.path,
    required this.name,
    required this.phase,
  });

  factory ProjectInfo.fromJson(Map<String, dynamic> json) => ProjectInfo(
        id: json['id'] as String? ?? '',
        path: json['path'] as String? ?? '',
        name: json['name'] as String? ?? '',
        phase: json['phase'] as String? ?? '',
      );
}

class ProjectState {
  final ProjectMeta project;
  final ProjectStatus status;
  final List<Decision> decisions;
  final List<Task> tasks;
  final List<Pitfall> pitfalls;

  ProjectState({
    required this.project,
    required this.status,
    required this.decisions,
    required this.tasks,
    required this.pitfalls,
  });

  factory ProjectState.fromJson(Map<String, dynamic> json) => ProjectState(
        project: ProjectMeta.fromJson(json['project'] as Map<String, dynamic>? ?? {}),
        status: ProjectStatus.fromJson(json['status'] as Map<String, dynamic>? ?? {}),
        decisions: (json['decisions'] as List<dynamic>?)
                ?.map((e) => Decision.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
        tasks: (json['tasks'] as List<dynamic>?)
                ?.map((e) => Task.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
        pitfalls: (json['pitfalls'] as List<dynamic>?)
                ?.map((e) => Pitfall.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
      );
}

class ProjectMeta {
  final String name;
  final String summary;
  final String stack;
  final String docsPath;

  ProjectMeta({
    required this.name,
    required this.summary,
    required this.stack,
    required this.docsPath,
  });

  factory ProjectMeta.fromJson(Map<String, dynamic> json) => ProjectMeta(
        name: json['name'] as String? ?? '',
        summary: json['summary'] as String? ?? '',
        stack: json['stack'] as String? ?? '',
        docsPath: json['docs_path'] as String? ?? '',
      );
}

class ProjectStatus {
  final String currentFocus;
  final String phase;
  final String lastSession;

  ProjectStatus({
    required this.currentFocus,
    required this.phase,
    required this.lastSession,
  });

  factory ProjectStatus.fromJson(Map<String, dynamic> json) => ProjectStatus(
        currentFocus: json['current_focus'] as String? ?? '',
        phase: json['phase'] as String? ?? '',
        lastSession: json['last_session'] as String? ?? '',
      );
}

class Decision {
  final String what;
  final String why;
  final String when;

  Decision({required this.what, required this.why, required this.when});

  factory Decision.fromJson(Map<String, dynamic> json) => Decision(
        what: json['what'] as String? ?? '',
        why: json['why'] as String? ?? '',
        when: json['when'] as String? ?? '',
      );
}

class Task {
  final String name;
  final String status;
  final String? notes;
  final List<String>? dependsOn;
  final String? blockedReason;

  Task({
    required this.name,
    required this.status,
    this.notes,
    this.dependsOn,
    this.blockedReason,
  });

  factory Task.fromJson(Map<String, dynamic> json) => Task(
        name: json['name'] as String? ?? '',
        status: json['status'] as String? ?? 'todo',
        notes: json['notes'] as String?,
        dependsOn: (json['depends_on'] as List<dynamic>?)?.cast<String>(),
        blockedReason: json['blocked_reason'] as String?,
      );
}

class Pitfall {
  final String what;
  final String fix;

  Pitfall({required this.what, required this.fix});

  factory Pitfall.fromJson(Map<String, dynamic> json) => Pitfall(
        what: json['what'] as String? ?? '',
        fix: json['fix'] as String? ?? '',
      );
}

class Session {
  final int iteration;
  final String timestamp;
  final String task;
  final String outcome;
  final String summary;
  final String? handoff;
  final List<String>? filesChanged;
  final List<String>? decisionsMade;

  Session({
    required this.iteration,
    required this.timestamp,
    required this.task,
    required this.outcome,
    required this.summary,
    this.handoff,
    this.filesChanged,
    this.decisionsMade,
  });

  factory Session.fromJson(Map<String, dynamic> json) => Session(
        iteration: json['iteration'] as int? ?? 0,
        timestamp: json['timestamp'] as String? ?? '',
        task: json['task'] as String? ?? '',
        outcome: json['outcome'] as String? ?? '',
        summary: json['summary'] as String? ?? '',
        handoff: json['handoff'] as String?,
        filesChanged: (json['files_changed'] as List<dynamic>?)?.cast<String>(),
        decisionsMade: (json['decisions_made'] as List<dynamic>?)?.cast<String>(),
      );
}
