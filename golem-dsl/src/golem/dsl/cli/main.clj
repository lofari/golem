(ns golem.dsl.cli.main
  "CLI entry point for golem-dsl."
  (:require [golem.dsl.engine.core :as engine]
            [golem.dsl.engine.events :as events]
            [golem.dsl.engine.snapshot :as snapshot]
            [golem.dsl.session.claude :as claude]
            [golem.dsl.registry :as registry]
            [golem.dsl.resolve :as resolve]
            [clojure.edn :as edn]
            [clojure.java.io :as io]
            [clojure.string :as str]))

(defn- load-agent-file [path]
  (load-file path))

(defn parse-args
  "Parse CLI arguments into a structured map."
  [args]
  (let [cmd (first args)]
    (case cmd
      "run"
      (let [agent-name (second args)
            rest-args (drop 2 args)]
        (loop [remaining (seq rest-args)
               result {:command :run
                       :agent agent-name
                       :goal nil
                       :state-dir "."
                       :state-file nil
                       :opts {}
                       :dry-run false}]
          (if-not remaining
            result
            (let [flag (first remaining)]
              (case flag
                "--goal"      (recur (nnext remaining) (assoc result :goal (second remaining)))
                "--state-dir" (recur (nnext remaining) (assoc result :state-dir (second remaining)))
                "--state"     (recur (nnext remaining) (assoc result :state-file (second remaining)))
                "--opt"       (let [opt-str (second remaining)
                                   [k v] (str/split opt-str #"=" 2)]
                               (recur (nnext remaining) (update result :opts assoc k v)))
                "--dry-run"   (recur (next remaining) (assoc result :dry-run true))
                ;; skip unknown flags
                (recur (next remaining) result))))))

      "list"    {:command :list}
      "compile" {:command :compile :args (rest args)}
      "inspect" {:command :inspect :args (rest args)}
      "runs"    {:command :runs}
      {:command :help})))

(defn- cmd-run-parsed [{:keys [agent goal state-dir state-file opts dry-run]}]
  (let [agent-path (resolve/resolve-agent agent (str state-dir "/.ctx/agents"))]
    (when-not agent-path
      (binding [*out* *err*]
        (println (str "Unknown agent: " agent)))
      (System/exit 1))

    (load-agent-file agent-path)

    (let [agent-key (keyword agent)
          initial-state (cond
                          goal {:goal goal}
                          state-file (edn/read-string (slurp state-file))
                          :else {})]
      (when dry-run
        (events/emit! :step-start {:step "dry-run" :iteration 0 :agent agent})
        (events/emit! :agent-done {:agent agent :outcome "dry-run" :total-steps 0})
        (System/exit 0))

      (let [adapter (claude/make-adapter {:golem-binary "golem"})
            result (engine/run agent-key initial-state
                              {:adapter adapter
                               :base-dir state-dir
                               :state-dir state-dir
                               :working-dir state-dir})]
        (binding [*out* *err*]
          (println "Run complete:" (:run-id result))
          (println "Final status:" (:status result))
          (println "State version:" (:version result)))))))

(defn- cmd-list []
  (doseq [a (resolve/list-agents nil)]
    (println (format "%-20s %s [%s]" (:name a) (:desc a) (name (:source a))))))

(defn cmd-compile [args]
  (let [agent-file (first args)]
    (load-agent-file agent-file)
    (doseq [[name agent] (registry/all-agents)]
      (println "Agent:" name)
      (println "Nodes:" (count (:nodes agent)))
      (println "Contracts valid:" (nil? (registry/validate name)))
      (println))))

(defn cmd-inspect [args]
  (let [target (first args)]
    (if-let [agent (registry/get-agent (keyword target))]
      (do (println "Agent:" (:name agent))
          (println "\nNodes:")
          (doseq [n (:nodes agent)]
            (println " " (:id n) "->" (:primitive n)
                     "reads:" (get-in n [:contract :reads])
                     "writes:" (get-in n [:contract :writes])))
          (println "\nEdges:")
          (doseq [e (:edges agent)]
            (println " " e)))
      ;; Try as run ID
      (let [version (when (second args) (Integer/parseInt (subs (second args) 2)))
            state (snapshot/load-state "." (keyword target) (or version 0))]
        (if state
          (clojure.pprint/pprint state)
          (println "Not found:" target))))))

(defn cmd-runs [_args]
  (let [runs-dir (io/file "." "runs")]
    (if (.exists runs-dir)
      (doseq [f (sort (.listFiles runs-dir))]
        (when (.isDirectory f)
          (println (.getName f))))
      (println "No runs found."))))

(defn -main [& args]
  (let [parsed (parse-args args)]
    (case (:command parsed)
      :run     (cmd-run-parsed parsed)
      :list    (cmd-list)
      :compile (cmd-compile (:args parsed))
      :inspect (cmd-inspect (:args parsed))
      :runs    (cmd-runs nil)
      :help    (do (println "Usage: golem-dsl <command> [args]")
                   (println "Commands: run, list, compile, inspect, runs")
                   (println)
                   (println "run <agent> --goal <goal> --state-dir <dir> [--opt key=val] [--dry-run]")
                   (println "list                List available agents")))))
