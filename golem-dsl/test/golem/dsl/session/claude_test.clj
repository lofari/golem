(ns golem.dsl.session.claude-test
  (:require [clojure.test :refer [deftest is testing]]
            [golem.dsl.session.protocol :as proto]
            [golem.dsl.session.claude :as claude]))

(deftest claude-adapter-builds-correct-command
  (let [adapter (claude/make-adapter {:golem-binary "golem"
                                      :sandbox true
                                      :plugin-dirs ["/path/to/plugins"]
                                      :max-turns 200})]
    (is (satisfies? proto/SessionAdapter adapter))
    (let [cmd (claude/build-command adapter "/tmp/prompt.md" "/work" {})]
      (is (some #(= "--sandbox" %) cmd))
      (is (some #(= "--prompt" %) cmd))
      (is (some #(= "/tmp/prompt.md" %) cmd))
      (is (some #(= "--plugin-dir" %) cmd))
      (is (some #(= "/path/to/plugins" %) cmd)))))

(deftest claude-adapter-without-sandbox
  (let [adapter (claude/make-adapter {:golem-binary "golem"})
        cmd (claude/build-command adapter "/tmp/p.md" "/work" {})]
    (is (not (some #(= "--sandbox" %) cmd)))
    (is (some #(= "--max-turns" %) cmd))))
